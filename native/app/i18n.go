// i18n.go mirrors src/ui/messages.rs: the (zh, en) message table and
// error presentation at the UI boundary. UI strings use T(zh, en); core
// errors go through ErrorMessage, which keeps diagnostics out of the UI.
package app

import "strings"

// messages is the (zh, en) table from messages.rs.
var messages = [][2]string{
	{"请输入标签名称", "Enter a tag name"},
	{"标签名称不能包含 /", "Tag names cannot contain /"},
	{"父标签不存在", "Parent tag no longer exists"},
	{"子标签下不能继续创建标签", "Nested tags cannot contain more tags"},
	{"同一级已经存在这个标签", "A tag with this name already exists here"},
	{"标签不存在", "Tag no longer exists"},
	{"请输入分组名称", "Enter a group name"},
	{"已经存在同名分组", "A group with this name already exists"},
	{"分组不存在", "Group no longer exists"},
	{"技能不存在", "Skill no longer exists"},
	{"请选择技能文件夹", "Choose a skill folder"},
	{"请选择一个文件夹", "Choose a folder"},
	{"请至少选择一个技能", "Select at least one skill"},
	{"这个文件夹中没有找到可用的技能", "No skills found in this folder"},
	{"这个地址中没有找到可用的技能", "No skills found at this address"},
	{"这个插件中没有找到可用的技能", "No skills found in this plugin"},
	{"请输入 skills.sh 或 GitHub 地址", "Enter a skills.sh or GitHub address"},
	{"没有识别到 skills.sh 或 GitHub 地址", "Enter a valid skills.sh or GitHub address"},
	{"请输入 Claude 插件名称", "Enter a Claude plugin name"},
	{"没有识别到 Claude 插件名称", "Enter a valid Claude plugin name"},
	{"所选目录中没有 SKILL.md", "No SKILL.md found in the selected folder"},
	{"仓库中没有找到 SKILL.md", "No SKILL.md found in this repository"},
	{"Claude 插件中没有找到技能", "No skills found in this Claude plugin"},
	{"Claude 插件中没有找到指定技能", "Skill not found in this Claude plugin"},
	{"此技能链接到原始目录，请在来源中更新", "Update this skill in its original folder"},
	{"Kitter 内置 Skill 会随 Kitter 自动更新", "This built-in skill updates with Kitter"},
	{"Kitter 内置 Skill 不能删除", "This built-in skill cannot be deleted"},
	{"Kitter 内置 Skill 固定显示在列表顶部", "This built-in skill stays at the top of the list"},
	{"这个技能本身就是仅手动触发", "This skill is already manual-only"},
	{"这个技能不是由 Kitter 设为仅手动触发", "This skill was not set as manual by Kitter"},
	{"这个技能没有可用的更新来源", "No update source is available for this skill"},
	{"该文件不是可预览的文本文件", "This file cannot be previewed as text"},
	{"文件不在技能目录内", "Choose a file inside the skill folder"},
	{"技能名称无效", "Invalid skill name"},
	{"无法确定技能名称", "Could not read the skill name"},
	{"来源记录不一致", "Skill sources do not match"},
	{"Npx 扫描结果不可用，请重新扫描", "Scan the source again before adding skills"},
	{"引用已变化，请重新扫描", "The skill location changed. Scan again"},
	{"找不到用户目录", "Home folder could not be found"},
	{"无法确定用户目录", "Home folder could not be found"},
	{"已取消扫描", "Scan cancelled"},
}

// T picks the string for the app's language, as KitterApp::tr does.
func (a *App) T(zh, en string) string {
	if a.UsesEnglish() {
		return en
	}
	return zh
}

// ErrorMessage is messages::error_message.
func (a *App) ErrorMessage(err error) string {
	return ErrorMessageText(err.Error(), a.UsesEnglish())
}

// ErrorMessageText is the free function error_message in messages.rs.
func ErrorMessageText(message string, english bool) string {
	trimmed := strings.TrimSpace(message)
	for _, pair := range messages {
		if pair[0] == trimmed {
			if english {
				return pair[1]
			}
			return pair[0]
		}
	}
	var zh, en string
	switch {
	case strings.HasPrefix(message, "技能已变化") ||
		strings.HasPrefix(message, "引用已变化") ||
		strings.HasPrefix(message, "引用验证失败"):
		zh, en = "技能位置或内容已变化，请重新扫描", "The skill changed. Scan again"
	case strings.HasPrefix(message, "没有找到技能"):
		zh, en = "没有找到这个技能，请刷新后重试", "Skill not found. Refresh and try again"
	case strings.HasPrefix(message, "这个来源中的技能已添加"):
		zh, en = "这个技能已经添加", "This skill has already been added"
	case strings.HasPrefix(message, "发现多个名为"):
		zh, en = "发现同名技能，请调整名称后重试", "Some skills share a name. Rename them and try again"
	case strings.HasPrefix(message, "没有从来源中找到技能"):
		zh, en = "没有从这个来源中找到技能", "No skills found in this source"
	case strings.HasPrefix(message, "无法启动"):
		zh, en = "所需工具不可用，请确认已安装后重试", "A required tool is unavailable. Check that it is installed and try again"
	case strings.HasPrefix(message, "命令执行失败"):
		zh, en = "无法获取技能，请检查来源和网络连接后重试", "Could not fetch skills. Check the source and your connection, then try again"
	case strings.HasPrefix(message, "部分技能检查失败"):
		zh, en = "部分技能无法检查更新，请稍后重试", "Some skills could not be checked for updates. Try again later"
	default:
		zh, en = "操作未完成，请重试", "The operation could not be completed. Please try again"
	}
	if english {
		return en
	}
	return zh
}
