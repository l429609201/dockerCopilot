package schedule

import (
	"context"

	"github.com/google/uuid"
	"github.com/onlyLTY/dockerCopilot/internal/module/appconfig"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// ScheduleLogic 承载定时更新规则的增删改查与立即执行。
type ScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ScheduleLogic {
	return &ScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// List 返回所有定时更新规则。
func (l *ScheduleLogic) List() (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	cfg := l.svcCtx.AppConfig.Get()
	resp.Data = cfg.ScheduledUpdates
	return resp, nil
}

// Save 新建或更新一条规则；ID 为空则新建。保存后触发调度重载。
func (l *ScheduleLogic) Save(req *types.ScheduledRuleReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if req.Name == "" {
		resp.Code = 400
		resp.Msg = "规则名称不能为空"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	rule := appconfig.ScheduledUpdateRule{
		ID:               req.ID,
		Name:             req.Name,
		Enabled:          req.Enabled,
		Cron:             req.Cron,
		ContainerNames:   req.ContainerNames,
		OnlyWhenUpdate:   req.OnlyWhenUpdate,
		SkipInvalidTag:   req.SkipInvalidTag,
		RegistryID:       req.RegistryID,
		KeepOldContainer: req.KeepOldContainer,
		NotifyOnStart:    req.NotifyOnStart,
		NotifyOnDone:     req.NotifyOnDone,
		NotifyOnError:    req.NotifyOnError,
	}
	updateErr := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		if rule.ID == "" {
			rule.ID = uuid.New().String()
			cfg.ScheduledUpdates = append(cfg.ScheduledUpdates, rule)
			return nil
		}
		for i := range cfg.ScheduledUpdates {
			if cfg.ScheduledUpdates[i].ID == rule.ID {
				// 保留历史执行结果字段
				rule.LastRunAt = cfg.ScheduledUpdates[i].LastRunAt
				rule.LastResult = cfg.ScheduledUpdates[i].LastResult
				cfg.ScheduledUpdates[i] = rule
				return nil
			}
		}
		cfg.ScheduledUpdates = append(cfg.ScheduledUpdates, rule)
		return nil
	})
	if updateErr != nil {
		resp.Code = 500
		resp.Msg = "保存失败：" + updateErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	l.reloadScheduler()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"id": rule.ID}
	return resp, nil
}

// Delete 删除指定规则并触发调度重载。
func (l *ScheduleLogic) Delete(req *types.ScheduledRuleIDReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	found := false
	updateErr := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		kept := cfg.ScheduledUpdates[:0]
		for _, r := range cfg.ScheduledUpdates {
			if r.ID == req.ID {
				found = true
				continue
			}
			kept = append(kept, r)
		}
		cfg.ScheduledUpdates = kept
		return nil
	})
	if updateErr != nil {
		resp.Code = 500
		resp.Msg = "删除失败：" + updateErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	if !found {
		resp.Code = 400
		resp.Msg = "规则不存在"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	l.reloadScheduler()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{}
	return resp, nil
}

// RunNow 立即执行一条规则。
func (l *ScheduleLogic) RunNow(req *types.ScheduledRuleIDReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if l.svcCtx.Scheduler == nil || !l.svcCtx.Scheduler.RunNowByID(req.ID) {
		resp.Code = 400
		resp.Msg = "规则不存在或调度器未就绪"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "已触发执行"
	resp.Data = map[string]interface{}{}
	return resp, nil
}

// GetCron 返回全局定时更新 cron 表达式。
func (l *ScheduleLogic) GetCron() (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	cfg := l.svcCtx.AppConfig.Get()
	cron := cfg.ScheduledUpdateCron
	if cron == "" {
		cron = "30 4 * * *"
	}
	resp.Data = map[string]string{"cron": cron}
	return resp, nil
}

// SaveCron 更新全局定时更新 cron 表达式并触发调度重载。
func (l *ScheduleLogic) SaveCron(req *types.CronConfigReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if req.Cron == "" {
		resp.Code = 400
		resp.Msg = "cron 表达式不能为空"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	updateErr := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		cfg.ScheduledUpdateCron = req.Cron
		return nil
	})
	if updateErr != nil {
		resp.Code = 500
		resp.Msg = "保存失败：" + updateErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	l.reloadScheduler()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{}
	return resp, nil
}

// reloadScheduler 在配置变更后触发调度器重载。
func (l *ScheduleLogic) reloadScheduler() {
	if l.svcCtx.Scheduler != nil {
		l.svcCtx.Scheduler.Reload()
	}
}
