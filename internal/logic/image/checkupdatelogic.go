package image

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

type CheckUpdateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCheckUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckUpdateLogic {
	return &CheckUpdateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CheckUpdate 手动触发一轮镜像更新检测。
// 获取所有已启用主机的镜像列表，然后异步执行检测并把进度写入任务中心，
// 前端可凭返回的 taskID 在任务中心实时看到检测进度（定时检测不建任务，故不会刷屏）。
func (l *CheckUpdateLogic) CheckUpdate() (resp *types.Resp, err error) {
	resp = &types.Resp{}

	// 获取所有已启用主机的镜像列表（覆盖远程主机）
	images, err := utiles.GetAllImagesList(l.svcCtx)
	if err != nil {
		resp.Code = 500
		resp.Msg = "获取镜像列表失败: " + err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}

	taskID := uuid.New().String()
	total := len(images)
	// 先落一条初始进度，保证前端拿到 taskID 时任务中心已有该任务，避免短暂"任务不存在"
	l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
		TaskID:     taskID,
		Percentage: 0,
		Name:       "检查镜像更新",
		Message:    "正在准备检测",
		DetailMsg:  fmt.Sprintf("共 %d 个镜像待检测", total),
		TaskType:   svc.TaskTypeImageCheck,
	})

	// 异步执行：检测耗时随镜像数量增长，接口需立即返回 taskID 供前端订阅进度
	go l.runCheck(taskID, images)

	resp.Code = 200
	resp.Msg = "更新检测已触发，可在任务中心查看进度"
	resp.Data = map[string]interface{}{
		"imageCount": total,
		"taskID":     taskID,
	}
	return resp, nil
}

// runCheck 在后台执行检测并持续上报进度。
// 检测本身只有"完成 N / 共 M"这一维度可量化，故百分比直接按完成比例换算。
func (l *CheckUpdateLogic) runCheck(taskID string, images []types.Image) {
	defer func() {
		// 后台 goroutine panic 不能带崩进程，且必须把任务收尾为失败态，避免前端进度永久卡住
		if r := recover(); r != nil {
			logx.Errorf("镜像更新检测 panic 已恢复: %v", r)
			l.finishCheck(taskID, svc.TaskProgress{
				TaskID:     taskID,
				Percentage: 100,
				Name:       "检查镜像更新",
				Message:    "检测异常终止",
				DetailMsg:  fmt.Sprintf("%v", r),
				TaskType:   svc.TaskTypeImageCheck,
				Failed:     true,
			})
		}
	}()

	executed := l.svcCtx.HubImageInfo.CheckUpdateWithProgress(images, func(p module.CheckProgress) {
		pct := 0
		if p.Total > 0 {
			pct = p.Done * 100 / p.Total
			if pct > 99 {
				pct = 99 // 留最后 1% 给收尾，避免视觉上先到 100 再跳完成
			}
		}
		l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
			TaskID:     taskID,
			Percentage: pct,
			Name:       "检查镜像更新",
			Message:    fmt.Sprintf("正在检测 %d/%d", p.Done, p.Total),
			DetailMsg:  p.Current,
			TaskType:   svc.TaskTypeImageCheck,
		})
	})

	// 已有检查在进行中：本轮被跳过，明确告知用户而不是显示一个瞬间完成的空任务
	if !executed {
		l.finishCheck(taskID, svc.TaskProgress{
			TaskID:     taskID,
			Percentage: 100,
			Name:       "检查镜像更新",
			Message:    "已有检测在进行中",
			DetailMsg:  "本轮已跳过，请等待当前检测完成后再试",
			TaskType:   svc.TaskTypeImageCheck,
		})
		return
	}

	// 从内存检查结果中收集本轮「可更新」的镜像清单（多主机同名镜像去重），
	// 随完成态一并下发，供任务中心展开显示，无需前端再刷列表逐个比对。
	updatable := l.collectUpdatableImages(images)
	detail := fmt.Sprintf("已检测 %d 个镜像，%d 个可更新", len(images), len(updatable))
	if len(updatable) == 0 {
		detail = fmt.Sprintf("已检测 %d 个镜像，均为最新", len(images))
	}

	l.finishCheck(taskID, svc.TaskProgress{
		TaskID:          taskID,
		Percentage:      100,
		Name:            "检查镜像更新",
		Message:         "检测完成",
		DetailMsg:       detail,
		TaskType:        svc.TaskTypeImageCheck,
		UpdatableImages: updatable,
	})
}

// collectUpdatableImages 遍历本轮检测的镜像，按 image.ID 查内存检查结果，
// 收集 NeedUpdate=true 的镜像为可更新清单。多主机同名同 tag 镜像去重，
// 只关心「哪些镜像可更新」，不区分所属主机。
func (l *CheckUpdateLogic) collectUpdatableImages(images []types.Image) []svc.UpdatableImage {
	snapshot := l.svcCtx.HubImageInfo.Snapshot()
	seen := make(map[string]struct{})
	result := make([]svc.UpdatableImage, 0)
	for _, img := range images {
		r, ok := snapshot[img.ID]
		if !ok || !r.NeedUpdate {
			continue
		}
		key := img.ImageName + ":" + img.ImageTag
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, svc.UpdatableImage{ImageName: img.ImageName, ImageTag: img.ImageTag})
	}
	return result
}

// finishCheck 统一收尾，避免各分支重复写 IsDone。
func (l *CheckUpdateLogic) finishCheck(taskID string, p svc.TaskProgress) {
	p.IsDone = true
	l.svcCtx.UpdateProgress(taskID, p)
}
