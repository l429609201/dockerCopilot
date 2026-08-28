package utiles

import (
	"context"
	"fmt"
	"strings"

	ref "github.com/distribution/reference"
	"github.com/docker/docker/api/types/image"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	MyType "github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
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
	// seen 记录镜像ID -> 在 all 中的下标，便于去重后回填 InUsed
	seen := make(map[string]int)
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		list, err := GetImagesListFromHost(ctx, h.ID)
		if err != nil {
			logx.Errorf("聚合镜像列表跳过主机[%s:%s]: %v", h.ID, h.Name, err)
			continue
		}
		// 按镜像ID去重：不同主机上相同镜像 digest 相同，检查一次即可。
		// InUsed 取「任一主机在用即为在用」：同一镜像在 A 主机未用、B 主机有容器在用时，
		// 去重保留的那条必须反映 B 的在用状态，否则「有容器在用的可更新镜像」统计会漏项，
		// 与容器列表「有更新」角标对不上。
		for _, img := range list {
			if idx, ok := seen[img.ID]; ok {
				if img.InUsed {
					all[idx].InUsed = true
				}
				continue
			}
			seen[img.ID] = len(all)
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

// parseRepoTag 从 "repo:tag" 形式的引用中拆出镜像名与 tag。
//
// 不能用 strings.Split(ref, ":") 取 [0]/[1]：当 registry 带端口时
// （如 registry.local:5000/foo/bar:latest），第一个 ":" 是端口分隔符，
// 会把镜像名截成 "registry.local"、tag 截成 "5000/foo/bar"，
// 导致后续拼出的 manifest URL 必然 404。
// 这里用 Docker 官方 reference 库解析，正确处理端口、多级路径与省略的 registry。
func parseRepoTag(repoTag string) (name string, tag string, ok bool) {
	parsed, err := ref.ParseNormalizedNamed(repoTag)
	if err != nil {
		return "", "", false
	}
	// FamiliarName 保留用户书写习惯（官方镜像不加 docker.io/library/ 前缀），
	// 与前端展示、镜像名匹配逻辑保持一致。
	name = ref.FamiliarName(parsed)
	if tagged, isTagged := parsed.(ref.Tagged); isTagged {
		tag = tagged.Tag()
	}
	if tag == "" {
		tag = "latest"
	}
	return name, tag, true
}

// splitImageNameAndTag 填充每个镜像的 ImageName / ImageTag。
//
// RepoTags 可能有多条（同一镜像打了多个 tag），Docker 返回顺序不保证，
// 固定取 [0] 会挑到非预期的 tag。这里跳过 <none>:<none> 这类悬空 tag，
// 取第一条能正常解析的有效引用。
func splitImageNameAndTag(imagesList []MyType.Image) []MyType.Image {
	for i, imageInfo := range imagesList {
		imagesList[i].ImageName = "None"
		imagesList[i].ImageTag = "None"

		matched := false
		for _, repoTag := range imageInfo.RepoTags {
			// 悬空镜像的 RepoTags 是 "<none>:<none>"，解析没有意义
			if repoTag == "" || strings.Contains(repoTag, "<none>") {
				continue
			}
			if name, tag, ok := parseRepoTag(repoTag); ok {
				imagesList[i].ImageName = name
				imagesList[i].ImageTag = tag
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// 无可用 RepoTags 时退回 RepoDigests，只能拿到仓库名，tag 未知
		for _, repoDigest := range imageInfo.RepoDigests {
			repo := strings.SplitN(repoDigest, "@", 2)[0]
			if repo == "" || strings.Contains(repo, "<none>") {
				continue
			}
			if parsed, err := ref.ParseNormalizedNamed(repo); err == nil {
				imagesList[i].ImageName = ref.FamiliarName(parsed)
			} else {
				imagesList[i].ImageName = repo
			}
			imagesList[i].ImageTag = "None"
			break
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
