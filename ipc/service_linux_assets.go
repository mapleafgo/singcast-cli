//go:build linux

package ipc

// polkitRules 生成放行 resolved 三条 action 的 polkit 规则。
// 对本地登录用户无条件放行，使 core 以当前用户身份调 resolvectl 时不弹密码。
func polkitRules() string {
	return `polkit.addRule(function(action, subject) {
    if ((action.id == "org.freedesktop.resolve1.set-domains" ||
         action.id == "org.freedesktop.resolve1.set-default-route" ||
         action.id == "org.freedesktop.resolve1.set-dns-servers") &&
        subject.local == true) {
        return polkit.Result.YES;
    }
});
`
}
