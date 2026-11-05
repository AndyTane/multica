package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/brand"
	"github.com/multica-ai/multica/server/internal/managedkit"
)

// EngineDiagnostic contains references and repair instructions, never file contents.
type EngineDiagnostic struct {
	Provider     string `json:"provider"`
	State        string `json:"state"`
	ProgramPath  string `json:"program_path,omitempty"`
	ConfigPath   string `json:"config_path"`
	ConfigExists bool   `json:"config_exists"`
	Message      string `json:"message"`
	Repair       string `json:"repair"`
}

type ExecutionReadiness struct {
	State        string             `json:"state"`
	Message      string             `json:"message"`
	RuntimeCount int                `json:"runtime_count"`
	Diagnostics  []EngineDiagnostic `json:"diagnostics"`
}

type ConnectionStatus struct {
	State      string `json:"state"`
	VerifiedAt string `json:"verified_at,omitempty"`
}

// Only non-secret bridge file paths cross the inherited product-env filter.
func applyDshBridgePaths(env map[string]string) {
	for _, key := range []string{"MULTICA_DSH_WEB_CONFIG", "MULTICA_DSH_NODE_PATH"} {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}
}

func (d *Daemon) claimableRuntimeIDs() []string {
	d.mu.Lock()
	ids := make([]string, 0, len(d.runtimeIndex))
	for id := range d.runtimeIndex {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	kept := ids[:0]
	for _, id := range ids {
		rt := d.findRuntime(id)
		if rt == nil {
			continue
		}
		if rt.Provider == "opencode" && rt.ProfileID == "" && d.openCodeDiagnostic(d.agents()["opencode"].Path).State != "configured" {
			continue
		}
		if rt.Provider == "dsh" && rt.ProfileID == "" && !dshMulticaProfilePresent() {
			continue
		}
		kept = append(kept, id)
	}
	return kept
}

func (d *Daemon) openCodeRuntimeNeedsRepair() bool {
	d.mu.Lock()
	registered := false
	for _, rt := range d.runtimeIndex {
		if rt.Provider == "opencode" && rt.ProfileID == "" {
			registered = true
			break
		}
	}
	d.mu.Unlock()
	return registered && d.openCodeDiagnostic(d.agents()["opencode"].Path).State != "configured"
}

func (d *Daemon) connectionStatus() ConnectionStatus {
	stamp := d.connectionVerified.Load()
	if stamp == 0 {
		return ConnectionStatus{State: "connecting"}
	}
	state := "connected"
	if d.connectionFailed.Load() || time.Since(time.Unix(0, stamp)) > 3*d.connectionInterval() {
		state = "disconnected"
	}
	return ConnectionStatus{State: state, VerifiedAt: time.Unix(0, stamp).UTC().Format(time.RFC3339)}
}

func (d *Daemon) executionReadiness() ExecutionReadiness {
	n := len(d.claimableRuntimeIDs())
	r := ExecutionReadiness{State: "pending_configuration", Message: "待配置，暂不能接任务", RuntimeCount: n}
	if n > 0 {
		r.State = "ready"
		r.Message = "执行环境已就绪，可接任务（模型访问以实际执行结果为准）"
	}
	program := ""
	if e, ok := d.agents()["opencode"]; ok {
		program = e.Path
	}
	r.Diagnostics = append(r.Diagnostics, d.openCodeDiagnostic(program), d.dshDiagnostic(), d.dshWebDiagnostic())
	return r
}

func (d *Daemon) openCodeDiagnostic(program string) EngineDiagnostic {
	home, _ := os.UserHomeDir()
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	paths := []string{filepath.Join(configHome, "opencode", "opencode.jsonc"), filepath.Join(configHome, "opencode", "opencode.json")}
	if dir := os.Getenv("OPENCODE_CONFIG_DIR"); dir != "" {
		paths = []string{filepath.Join(dir, "opencode.jsonc"), filepath.Join(dir, "opencode.json")}
	}
	if file := os.Getenv("OPENCODE_CONFIG"); file != "" {
		paths = []string{file}
	}
	selected, selectionErr := managedkit.ReadOpenCodeSelection(os.Getenv("U_WORKERS_KIT_ROOT"), d.cfg.Profile)
	if selected != nil {
		program = selected.Executable
		if selected.ConfigPath != "" {
			paths = []string{selected.ConfigPath}
		}
	}
	path := paths[0]
	path, _ = filepath.Abs(path)
	r := EngineDiagnostic{Provider: "opencode", ProgramPath: program, ConfigPath: path,
		Repair: "OpenCode 与 DeepSeek Harness 任选其一。安装 OpenCode 后，在所示 JSON/JSONC 文件中配置所用模型和服务商，并在 OpenCode 中完成登录；任务空闲后 Stop.cmd，再运行 client.ps1 -Action Reconnect -OpenCodePath <程序完整路径> -OpenCodeConfig <配置完整路径> 重新检测。"}
	if selectionErr != nil || d.cfg.OpenCodeSelectionIssue != "" {
		r.State = "invalid_selection"
		r.ConfigPath = filepath.Join(os.Getenv("U_WORKERS_KIT_ROOT"), "opencode-selection.json")
		r.Message = "OpenCode 选择记录无法读取或路径无效；保留原文件，修正后重新选择。"
		return r
	}
	for _, p := range paths {
		if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
			r.State = "invalid_selection"
			r.Message = "OpenCode 配置必须使用本机完整路径；不会访问网络共享。"
			return r
		}
		if _, err := os.Stat(p); err == nil {
			path, _ = filepath.Abs(p)
			break
		}
	}
	r.ConfigPath = path
	_, existsErr := os.Stat(path)
	r.ConfigExists = existsErr == nil
	if selected != nil && selected.Disabled {
		r.State = "disabled"
		r.Message = "OpenCode 已由用户跳过；重新选择程序和配置后才会启用。"
		return r
	}
	if program == "" || !agentExecutablePresent(program) {
		r.State = "missing_program"
		r.Message = "缺少 OpenCode 程序，或所选程序已丢失；不会静默改用其他安装。"
		return r
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		r.State = "missing_config"
		r.Message = "缺少 OpenCode 配置；请在此完整路径创建配置，或明确选择已有文件。"
		return r
	}
	r.ConfigExists = err == nil
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
		r.State = "unreadable_config"
		r.Message = "所选 OpenCode 配置不可读、不是文件或超过 4 MiB；原选择保留。"
		return r
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		r.State = "unreadable_config"
		r.Message = "所选 OpenCode 配置不可读；请检查文件权限。"
		return r
	}
	cfg, err := unmarshalRuntimeMcpConfig(raw, "jsonc")
	if err != nil || cfg == nil {
		r.State = "invalid_config"
		r.Message = "OpenCode 配置格式错误；请修正 JSON/JSONC，配置内容未输出。"
		return r
	}
	r.State = "configured"
	r.Message = "OpenCode 配置可读且格式正确；未发起模型调用，模型访问以实际任务为准。"
	return r
}

func (d *Daemon) dshDiagnostic() EngineDiagnostic {
	home, _ := os.UserHomeDir()
	root := strings.TrimSpace(os.Getenv("DSH_HOME"))
	if root == "" {
		root = filepath.Join(home, ".dsh")
	}
	root, _ = filepath.Abs(root)
	r := EngineDiagnostic{Provider: "dsh", ConfigPath: filepath.Join(root, "profiles", "multica", "package.json"),
		Repair: "这是 DSH 的 U Workers 接入 profile 清单，不是模型配置。安装 DeepSeek Harness，在其设置中选择服务商、模型并登录，再按 DSH 接入说明安装 multica profile；不要手写空 package.json。任务空闲后 Stop.cmd → Reconnect.cmd 重新检测。"}
	if e, ok := d.agents()["dsh"]; ok {
		r.ProgramPath = e.Path
	}
	_, err := os.Stat(r.ConfigPath)
	r.ConfigExists = err == nil
	if r.ProgramPath == "" {
		r.State = "missing_program"
		r.Message = "未找到 DeepSeek Harness CLI；可安装 DSH，或选择 OpenCode。"
	} else if !r.ConfigExists {
		r.State = "missing_profile"
		r.Message = "DSH 程序已找到，但缺少 multica 接入 profile。"
	} else {
		r.State = "configured"
		r.Message = "DSH 接入清单存在；协议探测和模型设置仍需可用。"
	}
	return r
}

func (d *Daemon) dshWebDiagnostic() EngineDiagnostic {
	home, _ := os.UserHomeDir()
	path := strings.TrimSpace(os.Getenv("MULTICA_DSH_WEB_CONFIG"))
	if path == "" {
		path = filepath.Join(brand.ConfigRoot(home, func(p string) bool { _, e := os.Stat(p); return e == nil }), "dsh-web.json")
	}
	path, _ = filepath.Abs(path)
	_, err := os.Stat(path)
	return EngineDiagnostic{Provider: "dsh_web", State: "optional_bridge", ConfigPath: path, ConfigExists: err == nil,
		Message: "如使用 DSH 网页接入，这是 Web bridge 连接配置，与 CLI 接入 profile 和模型配置不同。",
		Repair:  fmt.Sprintf("保持 DSH 网页已启动；使用 scripts/configure-dsh-web.ps1 -WebOrigin <本机主页地址> -AccessLog <DSH 访问日志完整路径> -Profile %q 配置桥接、探测并绑定运行环境。文件仅含 origin 和 accessLog；模型仍在 DSH 设置中配置。之后空闲时 Stop.cmd → Reconnect.cmd。", d.cfg.Profile)}
}
