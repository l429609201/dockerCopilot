package schedule

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/schedule"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ListHandler 返回所有定时更新规则。
// ListHandler 获取定时规则列表
// @Summary 获取定时规则列表
// @Tags 定时任务
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /schedules [get]
func ListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := schedule.NewScheduleLogic(r.Context(), svcCtx)
		resp, err := l.List()
		writeResp(w, r, resp, err)
	}
}

// SaveHandler 新建或更新定时更新规则。
// SaveHandler 创建/更新定时规则
// @Summary 创建或更新定时规则
// @Tags 定时任务
// @Accept json
// @Produce json
// @Param body body types.ScheduledRuleReq true "规则配置"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /schedules [post]
func SaveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ScheduledRuleReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := schedule.NewScheduleLogic(r.Context(), svcCtx)
		resp, err := l.Save(&req)
		writeResp(w, r, resp, err)
	}
}

// DeleteHandler 删除定时更新规则。
// DeleteHandler 删除定时规则
// @Summary 删除定时规则
// @Tags 定时任务
// @Produce json
// @Param id path string true "规则ID"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /schedules/{id} [delete]
func DeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ScheduledRuleIDReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := schedule.NewScheduleLogic(r.Context(), svcCtx)
		resp, err := l.Delete(&req)
		writeResp(w, r, resp, err)
	}
}

// RunNowHandler 立即执行指定定时更新规则。
// RunNowHandler 立即执行定时规则
// @Summary 立即执行定时规则
// @Tags 定时任务
// @Produce json
// @Param id path string true "规则ID"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /schedules/{id}/run [post]
func RunNowHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ScheduledRuleIDReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := schedule.NewScheduleLogic(r.Context(), svcCtx)
		resp, err := l.RunNow(&req)
		writeResp(w, r, resp, err)
	}
}

// writeResp 统一响应写出：业务错误按 resp.Code 返回，否则 200。
func writeResp(w http.ResponseWriter, r *http.Request, resp *types.Resp, err error) {
	if err != nil {
		httpx.WriteJson(w, resp.Code, resp)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, resp)
}
