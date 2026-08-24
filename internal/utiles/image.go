package utiles

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/image"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	MyType "github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"strings"
)

// GetImagesList 获取本地 Docker 主机的镜像列表（保留原签名，兼容既有调用）。
func GetImagesList(ctx *svc.ServiceContext) ([]MyType.Image, error) {
	return GetImagesListFromHost(ctx, appconfig.DockerHostLocalID)
}

// GetImagesListFromHost 获取指定 Docker 主机的镜像列表。hostID 为空取本地主机。
// 返回的每条镜像都会标记其所属主机的 HostID / HostName，便于前端展示与操作路由。
func GetImagesListFromHost(ctx *svc.ServiceContext, hostID string) ([]MyType.Image, error) {
	var imagesList []MyType.Image
	if hostID == "" {
		hostID = appconfig.DockerHostLocalID
	}
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		return imagesList, fmt.Errorf("docker 主机 %s 无可用连接", hostID)
	}
	dockerImages, err := cli.ImageList(context.Background(), image.ListOptions{})
	if err != nil {
		// 不能用 log.Fatalf，否则获取镜像失败会直接杀掉整个进程
		logx.Errorf("Unable to fetch docker images (host=%s): %s", hostID, err)
		return imagesList, err
	}

	// 解析主机展示名，找不到回退主机ID
	hostName := hostID
	if h, ok := ctx.AppConfig.FindDockerHost(hostID); ok && h.Name != "" {
		hostName = h.Name
	}

	for _, img := range dockerImages {
		i := MyType.Image{
			Summary:    img,
			ImageName:  "",
			ImageTag:   "",
			InUsed:     false,
			SizeFormat: "",
			HostID:     hostID,
			HostName:   hostName,
		}
		imagesList = append(imagesList, i)
	}
	//看不明白就不要看了，这内存反复地申请，如果你看明白了 给这改成指针吧，啥？我为啥不直接写指针，我懒癌犯了就这样，欢迎pr
	imagesList, err = checkImageInUsedOnHost(ctx, hostID, splitImageNameAndTag(calculateImageSize(imagesList)))
	if err != nil {
		return imagesList, err
	}
	return imagesList, nil
}

// GetAllImagesList 聚合所有已启用 Docker 主机的镜像列表，按镜像ID去重。
// 用于更新检查：只有覆盖所有主机的镜像，远程主机容器才能正确显示"可更新"。
// 单个主机不可达仅记录日志并跳过，不影响其它主机。
func GetAllImagesList(ctx *svc.ServiceContext) ([]MyType.Image, error) {
	ctx.AppConfig.EnsureLocalHost()
	hosts := ctx.AppConfig.ListDockerHosts()
	var all []MyType.Image
	seen := make(map[string]struct{})
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		list, err := GetImagesListFromHost(ctx, h.ID)
		if err != nil {
			logx.Errorf("聚合镜像列表跳过主机[%s:%s]: %v", h.ID, h.Name, err)
			continue
		}
		// 按镜像ID去重：不同主机上相同镜像 digest 相同，检查一次即可
		for _, img := range list {
			if _, ok := seen[img.ID]; ok {
				continue
			}
			seen[img.ID] = struct{}{}
			all = append(all, img)
		}
	}
	return all, nil
}

// GetAllImagesListPerHost 聚合所有已启用 Docker 主机的镜像列表，**不去重**。
// 与 GetAllImagesList 的区别：同一镜像在多个主机上会各自列出一条（均带 HostID/HostName），
// 用于镜像管理页真实反映各主机的镜像占用与删除路由。
// 单个主机不可达仅记录日志并跳过，不影响其它主机。
func GetAllImagesListPerHost(ctx *svc.ServiceContext) ([]MyType.Image, error) {
	ctx.AppConfig.EnsureLocalHost()
	hosts := ctx.AppConfig.ListDockerHosts()
	var all []MyType.Image
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		list, err := GetImagesListFromHost(ctx, h.ID)
		if err != nil {
			logx.Errorf("按主机聚合镜像列表跳过主机[%s:%s]: %v", h.ID, h.Name, err)
			continue
		}
		all = append(all, list...)
	}
	return all, nil
}

func splitImageNameAndTag(imagesList []MyType.Image) []MyType.Image {
	for i, imageInfo := range imagesList {
		if len(imageInfo.RepoTags) != 0 {
			imagesList[i].ImageName = strings.Split(imageInfo.RepoTags[0], ":")[0]
			imagesList[i].ImageTag = strings.Split(imageInfo.RepoTags[0], ":")[1]
		} else if len(imageInfo.RepoDigests) != 0 {
			imagesList[i].ImageName = strings.Split(imageInfo.RepoDigests[0], "@")[0]
			imagesList[i].ImageTag = "None"
		} else {
			imagesList[i].ImageName = "None"
			imagesList[i].ImageTag = "None"
		}
	}
	return imagesList
}
func checkImageInUsed(svc *svc.ServiceContext, imageList []MyType.Image) ([]MyType.Image, error) {
	return checkImageInUsedOnHost(svc, appconfig.DockerHostLocalID, imageList)
}

// checkImageInUsedOnHost 用指定主机的容器列表判断镜像是否在用，避免跨主机误判 InUsed。
func checkImageInUsedOnHost(svc *svc.ServiceContext, hostID string, imageList []MyType.Image) ([]MyType.Image, error) {
	list, err := GetContainerListFromHost(svc, hostID)
	if err != nil {
		return imageList, err
	}
	// 这里可以用mapreduce 我懒等pr
	for _, v := range list {
		for i, imageInfo := range imageList {
			if v.ImageID == imageInfo.ID {
				imageList[i].InUsed = true
				break
			}
		}
	}
	return imageList, nil
}
func calculateImageSize(imagesList []MyType.Image) []MyType.Image {
	for i := range imagesList {
		if imagesList[i].Size >= 1024*1024*1024 {
			imagesList[i].SizeFormat = // Convert size to gigabytes
				fmt.Sprintf("%d Gb", imagesList[i].Size/1024/1024/1024)
		} else {
			imagesList[i].SizeFormat = // Convert size to megabytes
				fmt.Sprintf("%d Mb", imagesList[i].Size/1024/1024)
		}
	}
	return imagesList
}
