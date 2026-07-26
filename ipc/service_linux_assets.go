//go:build linux

package ipc

import (
	"os"
	"strconv"
)

// linuxFile 描述 install 时要写入的一个文件。
type linuxFile struct {
	Path    string
	Content string
	Mode    os.FileMode
}

// systemdUnit 生成 singcast-core.service 内容。
// execPath 为二进制真实路径，stateDir 为服务 state 目录，
// guiUID 为发起 install 的 GUI 用户 uid（用于 socket ACL）。
func systemdUnit(execPath, stateDir, guiUID string) string {
	quotedExe := strconv.Quote(execPath)
	quotedHome := strconv.Quote(stateDir)
	envGui := ""
	// 包安装以 root 跑时 guiUID 可能是 "0"；不写无效 ACL 目标，留给首次 GUI 刷新。
	if guiUID != "" && guiUID != "0" {
		envGui = "Environment=SINGCAST_GUI_UID=" + guiUID + "\n"
	}
	// Bounding 与 Ambient 对齐到 TUN/DNS 所需网络能力；不做 ptrace/dac 扩大面。
	return `[Unit]
Description=Singcast core service
After=network-online.target nss-lookup.target
Wants=network-online.target

[Service]
Type=simple
User=` + serviceUser + `
Group=` + serviceUser + `
StateDirectory=singcast
RuntimeDirectory=singcast
RuntimeDirectoryMode=0755
WorkingDirectory=` + stateDir + `
Environment=SINGCAST_IPC_PATH=/run/singcast/command.sock
` + envGui + `ExecStart=` + quotedExe + ` ipc --home ` + quotedHome + `
AmbientCapabilities=CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE
Restart=on-failure
RestartSec=2
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
`
}

// polkitRules 生成放行 resolved 相关 action（对 singcast 用户），
// 以及允许本地 active 用户启停 singcast-core.service 的 polkit 规则。
// 含 set-* 与 revert：上游 sing-tun 关 TUN 时会 resolvectl revert。
func polkitRules() string {
	return `polkit.addRule(function(action, subject) {
    if ((action.id == "org.freedesktop.resolve1.set-domains" ||
         action.id == "org.freedesktop.resolve1.set-default-route" ||
         action.id == "org.freedesktop.resolve1.set-dns-servers" ||
         action.id == "org.freedesktop.resolve1.revert") &&
        subject.user == "` + serviceUser + `") {
        return polkit.Result.YES;
    }
});

polkit.addRule(function(action, subject) {
    if (action.id == "org.freedesktop.systemd1.manage-units" &&
        subject.active == true && subject.local == true &&
        action.lookup("unit") == "` + LinuxServiceName + `.service") {
        // 不按 verb 过滤：细节字段在发行版间不稳定；已用 unit 名收窄到本服务。
        return polkit.Result.YES;
    }
});
`
}

// linuxInstallFiles 返回 install 时要写入的全部文件。
func linuxInstallFiles(execPath, stateDir, guiUID string) []linuxFile {
	return []linuxFile{
		{Path: "/etc/systemd/system/" + LinuxServiceName + ".service", Content: systemdUnit(execPath, stateDir, guiUID), Mode: 0o644},
		{Path: "/usr/share/polkit-1/rules.d/singcast.rules", Content: polkitRules(), Mode: 0o644},
	}
}

// linuxUninstallPaths 返回 uninstall 时要删除的文件路径。
func linuxUninstallPaths() []string {
	return []string{
		"/etc/systemd/system/" + LinuxServiceName + ".service",
		"/usr/share/polkit-1/rules.d/singcast.rules",
		stableServiceBinaryPath(),
	}
}
