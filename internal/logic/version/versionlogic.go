package version

import (
	"context"

	"github.com/l429609201/dockerCopilot/internal/config"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

type VersionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewVersionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *VersionLogic {
	return &VersionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *VersionLogic) Version(req *types.VersionReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if req.Type == "local" {
		resp.Code = 200
		resp.Msg = "success"
		resp.Data = map[string]string{
			"version":   config.Version,
			"buildDate": config.BuildDate,
		}
		return resp, nil
	} else if req.Type == "remote" {
		// 二进制自更新已下线，不再远程比对版本。
		// 保留该分支仅为兼容旧前端：始终返回“无更新”，升级请更新容器镜像。
		resp.Code = 200
		resp.Msg = "程序无更新"
		resp.Data = map[string]string{
			"remoteVersion": config.Version,
		}
		return resp, nil
	} else {
		resp.Code = 400
		resp.Msg = "type 参数错误"
		resp.Data = map[string]string{}
		return resp, nil
	}
}
