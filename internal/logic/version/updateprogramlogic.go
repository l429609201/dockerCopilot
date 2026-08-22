package version

import (
	"context"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateProgramLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateProgramLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProgramLogic {
	return &UpdateProgramLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateProgram 二进制热替换自更新已下线。
// 现在统一通过“容器整体更新”（拉取新镜像重建容器）来升级，
// 此接口保留仅为兼容旧前端调用，直接返回提示不再执行任何替换。
func (l *UpdateProgramLogic) UpdateProgram() (resp *types.Resp, err error) {
	resp = &types.Resp{}
	resp.Code = 400
	resp.Msg = "二进制自更新已下线，请通过更新容器镜像进行升级"
	resp.Data = map[string]interface{}{}
	return resp, nil
}
