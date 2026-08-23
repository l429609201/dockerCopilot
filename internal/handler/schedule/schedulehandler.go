package schedule

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/schedule"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ListHandler 返回所有定时更新规则。
func ListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := schedule.NewScheduleLogic(r.Context(), svcCtx)
		resp, err := l.List()
		writeResp(w, r, resp, err)
	}
}

// SaveHandler 新建或更新定时更新规则。
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
