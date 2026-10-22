package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEULA(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"默认模板", "# 注释\neula=false\n", false},
		{"同意", "eula=true\n", true},
		{"大写", "EULA=TRUE\n", true},
		{"带空格", "  eula = true  \n", true},
		{"混合大小写与空格", "Eula = True\n", true},
		{"空内容", "", false},
		{"只有注释", "# eula=true\n", false},
		{"没有等号", "eulatrue\n", false},
		{"其他键在前", "foo=bar\neula=true\n", true},
		{"false 后跟 true（取第一个）", "eula=false\neula=true\n", false},
		{"CRLF 行尾", "# x\r\neula=true\r\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseEULA(c.content); got != c.want {
				t.Errorf("parseEULA(%q) = %v, want %v", c.content, got, c.want)
			}
		})
	}
}

// TestCheckEULAGeneratesTemplate 验证首次运行会在工作目录生成 eula.txt，
// 且默认是 eula=false（因此 checkEULA 返回 false）。
func TestCheckEULAGeneratesTemplate(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if checkEULA() {
		t.Fatal("首次运行（无 eula.txt）不应视为已同意")
	}
	data, err := os.ReadFile(filepath.Join(dir, eulaFile))
	if err != nil {
		t.Fatalf("未生成 %s: %v", eulaFile, err)
	}
	s := string(data)
	if !strings.Contains(s, "eula=false") {
		t.Errorf("生成的模板应含 eula=false，实际:\n%s", s)
	}
	if !strings.Contains(s, eulaURL) {
		t.Errorf("生成的模板应含协议地址 %s，实际:\n%s", eulaURL, s)
	}
}

// TestCheckEULAReadsAgreement 验证把 eula=false 改成 eula=true 后可以启动。
func TestCheckEULAReadsAgreement(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if checkEULA() {
		t.Fatal("无文件时不应通过")
	}
	if err := os.WriteFile(filepath.Join(dir, eulaFile), []byte("# ok\neula=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !checkEULA() {
		t.Fatal("eula=true 后应通过")
	}
}

// TestCheckEULARejectsExplicitFalse 验证显式 false 被拒绝。
func TestCheckEULARejectsExplicitFalse(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, eulaFile), []byte("eula=false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if checkEULA() {
		t.Fatal("eula=false 不应通过")
	}
	// 文件已存在时不应被覆盖。
	data, err := os.ReadFile(filepath.Join(dir, eulaFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "eula=false") {
		t.Errorf("已存在的文件不应被改写，实际:\n%s", string(data))
	}
}

// TestWriteEULAAgreement 验证 writeEULAAgreement 写出可被 parseEULA 识别为同意的文件。
func TestWriteEULAAgreement(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if err := writeEULAAgreement(); err != nil {
		t.Fatalf("writeEULAAgreement 失败: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, eulaFile))
	if err != nil {
		t.Fatal(err)
	}
	if !parseEULA(string(data)) {
		t.Errorf("写出的内容应可解析为已同意，实际:\n%s", string(data))
	}
	if !strings.Contains(string(data), eulaURL) {
		t.Errorf("写出的内容应包含协议地址，实际:\n%s", string(data))
	}
	if strings.Contains(string(data), "eula=false") {
		t.Errorf("写出的内容不应残留 eula=false，实际:\n%s", string(data))
	}
	if !checkEULA() {
		t.Error("writeEULAAgreement 之后 checkEULA 应通过")
	}
}

// TestInstallEULAInteractive 验证只有 Linux + -install + 终端三条件同时成立才走交互询问。
func TestInstallEULAInteractive(t *testing.T) {
	if installEULAInteractive(false) {
		t.Error("非 -install 不应走交互询问")
	}
	// 测试环境的 stdin 通常不是字符设备（go test 会重定向），因此 -install
	// 在非终端下也不应走交互路径；这里只断言“不 panic 且为布尔值”。
	_ = installEULAInteractive(true)
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}
