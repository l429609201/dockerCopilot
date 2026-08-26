package image

import (
	"context"

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
// 获取所有已启用主机的镜像列表，然后调用 HubImageInfo.CheckUpdate 进行检测。
// 检测过程是异步的（go routine + 并发控制），本接口立即返回，前端可通过容器列表的 haveUpdate 字段查看结果。
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

	// 触发更新检测（异步执行，内部有去重保护）
	l.svcCtx.HubImageInfo.CheckUpdate(images)

	resp.Code = 200
	resp.Msg = "更新检测已触发，请稍后刷新容器列表查看结果"
	resp.Data = map[string]interface{}{
		"imageCount": len(images),
	}
	return resp, nil
}
