package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
//
// 例外：用 `-install` 注册 systemd 服务时，如果当前是交互式终端，会直接询问
// “是否同意 (y/n)”，同意后自动写 eula=true 并继续安装，省去手动跑一次再改文件的步骤。

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
func checkEULA() bool { return checkEULAWithHint(true) }

// checkEULAWithHint 与 checkEULA 相同，但 hint 为 false 时不打印“怎么改文件”的提示，
// 用于 -install 场景：紧接着要交互询问，重复打印反而啰嗦。
func checkEULAWithHint(hint bool) bool {
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

	if hint {
		printEULAInstructions(absPath)
	}
	return false
}

// printEULAInstructions 打印“未同意 EULA”的提示与修改指引。
func printEULAInstructions(absPath string) {
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
	fmt.Fprintln(os.Stderr, "  提示：如果打算用 systemd 常驻（-install），直接在终端里执行 -install 即可，")
	fmt.Fprintln(os.Stderr, "        届时会询问是否同意，输入 y 会自动把 eula.txt 写成 eula=true。")
	fmt.Fprintln(os.Stderr, "")
}

// eulaInteractive 报告能否在终端里交互询问（stdin 是真实终端，而不是管道或 /dev/null）。
func eulaInteractive() bool { return stdinIsTerminal() }

// installEULAInteractive 报告 -install 场景是否应该走“现场询问是否同意”的路径：
// 仅 Linux（-install 只在那里可用）且 stdin 是交互式终端。
func installEULAInteractive(install bool) bool {
	return install && runtime.GOOS == "linux" && stdinIsTerminal()
}

// promptEULA 在终端里展示协议地址并询问是否同意。
// 用户输入 y 时把 eula.txt 写成 eula=true 并返回 true；输入 n 或读不到输入返回 false。
func promptEULA() bool {
	absPath, err := filepath.Abs(eulaFile)
	if err != nil {
		absPath = eulaFile
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  ============================================================")
	fmt.Fprintln(os.Stderr, "   注册为 systemd 服务前，需要先同意《NeteaseBedrockGateway 最终用户许可协议》。")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "   协议全文：%s\n", eulaURL)
	fmt.Fprintln(os.Stderr, "  ============================================================")
	fmt.Fprintln(os.Stderr, "")

	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "  是否同意以上协议？输入 y 同意，n 取消 (y/n): ")
		line, rerr := in.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if rerr != nil && answer == "" {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "  未能读取输入，已取消。")
			return false
		}
		switch answer {
		case "y", "yes":
			if werr := writeEULAAgreement(); werr != nil {
				fmt.Fprintf(os.Stderr, "  写入 %s 失败: %v\n", absPath, werr)
				return false
			}
			fmt.Fprintf(os.Stderr, "  已记录同意（%s 中 eula=true），继续安装。\n", absPath)
			return true
		case "n", "no":
			fmt.Fprintln(os.Stderr, "  未同意协议，已取消安装。")
			return false
		default:
			fmt.Fprintln(os.Stderr, "  请输入 y 或 n。")
		}
	}
}

// writeEULAAgreement 把 eula.txt 写成已同意状态（eula=true），保留模板中的说明与生成时间。
func writeEULAAgreement() error {
	content := fmt.Sprintf(eulaTemplate, eulaURL, time.Now().Format("2006-01-02 15:04:05"))
	content = strings.Replace(content, "eula=false", "eula=true", 1)
	return os.WriteFile(eulaFile, []byte(content), 0o644)
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
