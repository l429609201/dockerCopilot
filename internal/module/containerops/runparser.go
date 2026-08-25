package containerops

import (
	"fmt"
	"strings"
)

// ParseRunCommand 将一条 `docker run ...` 命令解析为 CreateSpec。
// 仅覆盖常用参数；遇到无法识别的参数返回错误，提示用户改用其它方式。
// 支持形式：-e KEY=V / -e=KEY=V / --env KEY=V / --env=KEY=V / 合并短选项 -it。
func ParseRunCommand(command string) (CreateSpec, error) {
	spec := CreateSpec{AutoPull: true, AutoStart: true}
	tokens, err := tokenize(command)
	if err != nil {
		return spec, err
	}
	// 跳过起始的 docker / run 关键字
	i := 0
	if i < len(tokens) && tokens[i] == "docker" {
		i++
	}
	if i < len(tokens) && (tokens[i] == "run" || tokens[i] == "create") {
		i++
	}
	if i >= len(tokens) {
		return spec, fmt.Errorf("命令为空或缺少 docker run")
	}

	// 需要取值的长选项集合
	valueOpts := map[string]bool{
		"--name": true, "--env": true, "--publish": true, "--volume": true,
		"--restart": true, "--network": true, "--net": true, "--label": true,
		"--mount": true, "--entrypoint": true, "--hostname": true, "--workdir": true,
	}
	// 明确忽略的开关（无副作用或与本平台创建流程无关）
	ignoreFlags := map[string]bool{
		"-d": true, "--detach": true, "-i": true, "--interactive": true,
		"-t": true, "--tty": true, "--rm": true, "-it": true, "-ti": true,
	}

	image := ""
	var cmd []string
	for i < len(tokens) {
		tok := tokens[i]
		if image != "" {
			// 镜像之后的一切都是容器启动命令
			cmd = append(cmd, tok)
			i++
			continue
		}
		if !strings.HasPrefix(tok, "-") {
			// 第一个非选项 token 即镜像
			image = tok
			i++
			continue
		}
		// 短取值选项紧贴值形式：-eKEY=V / -p8080:80 / -v/a:/b / -lk=v
		if !strings.HasPrefix(tok, "--") && len(tok) > 2 && strings.ContainsAny(string(tok[1]), "epvl") {
			if err := applyValueOpt(&spec, shortToLong(tok[:2]), tok[2:]); err != nil {
				return spec, err
			}
			i++
			continue
		}
		// 短取值选项空格分隔形式：-e KEY=V / -p 8080:80 / -v /a:/b / -l k=v
		if !strings.HasPrefix(tok, "--") && len(tok) == 2 && strings.ContainsAny(string(tok[1]), "epvl") {
			if i+1 >= len(tokens) {
				return spec, fmt.Errorf("参数 %s 缺少取值", tok)
			}
			if err := applyValueOpt(&spec, shortToLong(tok), tokens[i+1]); err != nil {
				return spec, err
			}
			i += 2
			continue
		}
		// 拆分 --opt=value 形式
		key, inlineVal, hasInline := tok, "", false
		if idx := strings.Index(tok, "="); idx != -1 && strings.HasPrefix(tok, "--") {
			key = tok[:idx]
			inlineVal = tok[idx+1:]
			hasInline = true
		}
		if ignoreFlags[key] {
			i++
			continue
		}
		if valueOpts[key] {
			val := inlineVal
			if !hasInline {
				if i+1 >= len(tokens) {
					return spec, fmt.Errorf("参数 %s 缺少取值", key)
				}
				val = tokens[i+1]
				i += 2
			} else {
				i++
			}
			if err := applyValueOpt(&spec, key, val); err != nil {
				return spec, err
			}
			continue
		}
		return spec, fmt.Errorf("暂不支持的参数：%s，请改用逐项或 Compose 方式", tok)
	}

	if image == "" {
		return spec, fmt.Errorf("未识别到镜像名")
	}
	spec.Image = image
	spec.Cmd = cmd
	return spec, nil
}

// shortToLong 将短选项映射为长选项键。
func shortToLong(short string) string {
	switch short {
	case "-e":
		return "--env"
	case "-p":
		return "--publish"
	case "-v":
		return "--volume"
	case "-l":
		return "--label"
	}
	return short
}

// applyValueOpt 将一个已取到值的选项写入 spec。
func applyValueOpt(spec *CreateSpec, key, val string) error {
	val = strings.TrimSpace(val)
	if val == "" {
		return fmt.Errorf("参数 %s 的取值为空", key)
	}
	switch key {
	case "--name":
		spec.Name = val
	case "--env":
		spec.Env = append(spec.Env, val)
	case "--publish":
		spec.PortBindings = append(spec.PortBindings, val)
	case "--volume":
		spec.Binds = append(spec.Binds, val)
	case "--restart":
		spec.RestartPolicy = val
	case "--network", "--net":
		spec.NetworkMode = val
	case "--label":
		if spec.Labels == nil {
			spec.Labels = map[string]string{}
		}
		k, v, _ := strings.Cut(val, "=")
		spec.Labels[strings.TrimSpace(k)] = strings.TrimSpace(v)
	case "--entrypoint":
		spec.Entrypoint = append(spec.Entrypoint, val)
	case "--mount", "--hostname", "--workdir":
		// 这些参数当前创建流程未落地，忽略并不报错（避免因常见参数中断）
		return nil
	default:
		return fmt.Errorf("暂不支持的参数：%s", key)
	}
	return nil
}

// tokenize 将命令行拆分为 token，支持单/双引号包裹与反斜杠续行。
func tokenize(command string) ([]string, error) {
	// 先去掉 shell 续行符 "\<换行>"，把多行命令拼成一行
	command = strings.ReplaceAll(command, "\\\r\n", " ")
	command = strings.ReplaceAll(command, "\\\n", " ")

	var tokens []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range command {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t' || r == '\n' || r == '\r') && !inSingle && !inDouble:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("命令中的引号未闭合")
	}
	flush()
	return tokens, nil
}
