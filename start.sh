#!/bin/sh
cd "${WORKDIR:-/app}" || exit 1
# 判断当前目录下是否存在名为 dockerCopilot-new 的二进制文件
if [ -f "./dockerCopilot-new" ]; then
    # 如果存在，则用它覆盖 dockerCopilot
    mv ./dockerCopilot-new ./dockerCopilot
    # 赋予 dockerCopilot 执行权限
    chmod +x ./dockerCopilot
fi

# 直接交接进程，让容器正确接收停止信号。
exec ./dockerCopilot
