package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EULA（最终用户许可协议）检查。
//
// 行为与 Minecraft 服务端保持一致：
//   - 程序启动时查找工作目录下的 eula.txt；
//   - 文件不存在则自动生成，其中 eula=false；
//   - 只有显式写成 eula=true 才视为同意，否则输出提示并退出。
//
// 协议全文放在项目网站上，eula.txt 里只给出链接，避免每次升级都要改文本。

const (
	// eulaURL 是《最终用户许可协议》全文地址。
	eulaURL = "https://eula.redstone.xin/neteasegateway"
	// eulaFile 是协议确认文件名（位于程序工作目录）。
	eulaFile = "eula.txt"
)

// eulaTemplate 是首次运行时生成的 eula.txt 内容。
const eulaTemplate = `# 将下面一行改成 eula=true，表示你已阅读并同意《NeteaseBedrockGateway 最终用户许可协议》。
# 协议全文：%s
# 生成时间：%s
eula=false
`

// checkEULA 检查用户是否已同意最终用户许可协议。
// 返回 true 表示已同意；false 表示未同意或无法确认（调用方应退出）。
func checkEULA() bool {
	path := eulaFile
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			fmt.Fprintf(os.Stderr, "[EULA] 读取 %s 失败: %v\n", absPath, readErr)
		} else {
			// 首次运行：生成模板文件。
			content := fmt.Sprintf(eulaTemplate, eulaURL, time.Now().Format("2006-01-02 15:04:05"))
			if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
				fmt.Fprintf(os.Stderr, "[EULA] 生成 %s 失败: %v\n", absPath, werr)
			} else {
				fmt.Fprintf(os.Stderr, "[EULA] 首次运行，已生成 %s（默认 eula=false）\n", absPath)
			}
		}
	}

	if parseEULA(string(data)) {
		return true
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  ============================================================")
	fmt.Fprintln(os.Stderr, "   你还没有同意《NeteaseBedrockGateway 最终用户许可协议》。")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "   协议全文：%s\n", eulaURL)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "   阅读并同意后，请把 %s 中的\n", absPath)
	fmt.Fprintln(os.Stderr, "     eula=false")
	fmt.Fprintln(os.Stderr, "   改成")
	fmt.Fprintln(os.Stderr, "     eula=true")
	fmt.Fprintln(os.Stderr, "   然后重新启动本程序。")
	fmt.Fprintln(os.Stderr, "  ============================================================")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  提示：如果打算用 systemd 常驻（-install），请先在当前目录完成上面的确认，")
	fmt.Fprintln(os.Stderr, "        再执行 -install —— 服务的 WorkingDirectory 就是当前目录，")
	fmt.Fprintln(os.Stderr, "        启动时读的是同一份 eula.txt。")
	fmt.Fprintln(os.Stderr, "")
	return false
}

// parseEULA 解析 eula.txt 内容，返回是否明确写有 eula=true。
// 忽略空行与以 # 开头的注释行；键名与值都不区分大小写，允许等号两侧有空格。
func parseEULA(content string) bool {
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "eula") {
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}
