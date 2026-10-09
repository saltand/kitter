# Kitter 用 Go + MyGo 原生 UI 重写方案（macOS）

> 基线：`feat/manual-only`（`c6ae9d7`），工作分支：`feat/mygo-native-macos`
> 框架：[egoist/mygo](https://github.com/egoist/mygo)（v0.3.x，`ui` 包原生 GPU 渲染，纯 Go 无 cgo）
> 目的：试验 MyGo 原生 UI，核心、CLI、桌面端**全部**用 Go 重写，只做 macOS。

## 1. 目标与范围

- 功能对齐当前 Rust 版本：技能库、添加/采纳（Local / npx skills / Claude 插件 / 已有安装）、项目安装与卸载、有效技能扫描与上下文估算、分组、标签、manual-only、更新检查、设置（语言/主题）。
- 产物：
  - `Kitter.app` + `.dmg`（`go tool mygo build`）
  - `kitter` 独立 CLI（同一份核心代码，`go build ./cmd/kitter`）
- **数据完全兼容**：Go 版读写与 Rust 版相同的数据目录和文件格式，两个版本可以对同一个库交替运行。
- 不在范围：Windows / Linux（代码不刻意排斥，但不做适配和打包）。

## 2. 现状盘点（Rust，约 2.5 万行）

| 层 | 文件 | 行数 |
|---|---|---|
| 核心 | `effective_skills.rs` + `effective_skills/{adapters,catalog,scanner}.rs` | ~4.9k |
| 核心 | `library.rs` / `adoption.rs` / `source.rs` / `project.rs` | ~4.5k |
| 核心 | `agents.rs` / `tags.rs` / `config.rs` / `model.rs` / `directory_link.rs` / `text.rs` | ~1.3k |
| CLI | `bin/kitter.rs`（clap，多数子命令支持 `--json`） | 1.3k |
| UI | `src/ui/*.rs`（GPUI + gpui-component） | 12k |
| 平台 | `platform.rs`（NSVisualEffectView、软阴影子窗口）、`assets.rs`、`bin/kitter-desktop.rs` | ~0.5k |
| 测试 | `#[test]` / `#[gpui::test]` | ~124 个 |

磁盘数据（`app_data_dir()` = `$KITTER_HOME` 或 `~/Library/Application Support/Kitter`）：
`config.json`、`registry.json`、`tags.json`，以及库内各技能目录；项目侧涉及 `skills-lock.json`、各 Agent 的配置文件（`settings.json`、`opencode.json`、`openclaw.json`、`config.yaml`、`trust.json`、`package.json` 等）。

## 3. 依赖映射

| Rust | Go |
|---|---|
| gpui / gpui-component | `github.com/egoist/mygo` + `mygo/ui` |
| objc2 / NSVisualEffectView / 软阴影 | `WindowOptions.Vibrancy` + `TitleBarHiddenInset`（软阴影先不复刻） |
| fix-path-env | 自己实现：启动时执行 `$SHELL -ilc 'printf %s "$PATH"'` 合并进 `PATH` |
| serde / serde_json | `encoding/json`（字段名用 struct tag 对齐 serde 的 `rename_all`） |
| serde_yaml | `gopkg.in/yaml.v3` |
| clap | 标准库 `flag` + 手写子命令分发（子命令不多，不引入 cobra） |
| dirs | `os.UserHomeDir`，数据目录 `~/Library/Application Support/Kitter` |
| sys-locale | `mygo.App.Locale()`（CLI 用 `LANG`/`AppleLanguages`） |
| walkdir | `filepath.WalkDir` |
| ignore（gitignore 感知遍历） | `github.com/sabhiram/go-gitignore` 或自写最小实现（只需 `.gitignore` + 隐藏目录规则） |
| tempfile | `os.MkdirTemp` / `os.CreateTemp` |
| anyhow | `error` + `fmt.Errorf("%w")` |
| rust-embed（assets） | `//go:embed`（SVG 图标、PNG Agent 图标、内置 `resources/skills/kitter`）；JetBrains Mono 用 `ui.RegisterFont` 注册 |

## 4. 目录结构

在仓库内新增独立 Go module，和 Rust 代码并存，互不影响：

```
native/
├── go.mod                  # module github.com/saltand/kitter/native，go 1.27.1，依赖 mygo（锁定到具体版本）
├── mygo.json               # name: Kitter, identifier, version, icon
├── resources/
│   (icon 直接引用 ../assets/macos/app-icon.png，不复制)
│   └── skills/kitter/…     # 内置技能（也可 go:embed）
├── main.go                 # 桌面入口：PATH 修复、App.Run、菜单、窗口
├── cmd/kitter/main.go      # CLI 入口
├── core/                   # 纯逻辑，无 UI 依赖
│   ├── model/              # SkillOrigin, SkillSource, SkillRecord, SkillGroup, InstallTarget…
│   ├── config/             # AppConfig, app_data_dir, save_json（原子写）
│   ├── library/            # SkillLibrary：registry、import/adopt/replace/remove、分组、frontmatter
│   ├── source/             # scan_local / scan_npx / scan_claude、update、check_updates
│   ├── adoption/           # 扫描已有安装、引用替换与回滚
│   ├── project/            # install / uninstall / list / removal_kind
│   ├── agents/             # 各 Agent 安装根目录、图标信息
│   ├── effective/          # 有效技能扫描、adapters、catalog、上下文估算
│   ├── tags/               # TagState
│   └── fslink/             # directory_link（macOS 只需 symlink）
├── internal/testdata/      # 从 Rust 测试搬来的 fixture
└── app/                    # UI
    ├── app.go              # AppModel：所有状态的根，View(c)
    ├── shell.go            # 标题栏、侧栏导航、Split 布局、toast
    ├── theme.go            # 颜色、字号、间距常量（对齐 src/ui/theme.rs）
    ├── i18n.go             # 中英文消息表（对齐 src/ui/messages.rs 与界面文案）
    ├── icons.go            # go:embed SVG/PNG
    ├── skills_page.go
    ├── projects_page.go
    ├── settings_page.go
    ├── dialogs/            # add / install / delete / tags / assign_tags / groups / delete_group / move_group
    └── widgets/            # badges、selectable text、agent 图标行等复用块
```

## 5. 核心层移植

### 5.1 原则

- **按模块逐个翻译**，保持函数名和语义一一对应（`library.rs::adopt` → `library.(*Library).Adopt`），方便对照 diff。
- **测试先行**：每个模块先搬 Rust 的单元测试（fixture 放 `internal/testdata`），再写实现。
- **格式兼容**：所有 JSON 结构用 struct tag 对齐 serde 输出；枚举实现 `MarshalJSON/UnmarshalJSON`（如 `InstallTarget` 的 snake_case：`claude_code`、`open_code`…，以 Rust 实际序列化结果为准）。加一组「Rust 生成的数据 → Go 读取 → 再写回」的 round-trip 测试。
- 外部命令（`git`、`npx`）走 `exec.Command`，路径来自修复后的 `PATH`。
- 可取消的长任务（采纳扫描、项目扫描）用 `context.Context` 取代 `Arc<AtomicBool>`。

### 5.2 顺序（依赖从下到上）

1. `model` → `config` → `fslink` → `agents` → `tags`
2. `library`（含 frontmatter 解析、`validate_name`、分组、manual-only 标记）
3. `project`
4. `source`（含 `skills-lock.json`、npx/Claude 插件来源、更新检查）
5. `adoption`
6. `effective`（最大的模块，按 `scanner` → `adapters` → `catalog` → 主文件拆分）

### 5.3 内置技能

`library.rs` 在打开库时调用 `ensure_builtin_skill`，把 `resources/skills/kitter`（存储名 `_kitter-builtin`）写入库目录。Go 版用 `//go:embed` 嵌入同一份文件，行为一致；`native/` 通过构建前复制或相对 embed 引用仓库根的 `resources/skills/kitter`（Go embed 不能引用 module 外的路径，所以 M0 用脚本同步到 `native/core/library/builtin/`，并加测试保证与源文件一致）。

### 5.4 CLI

`cmd/kitter` 覆盖现有子命令：`list / show / files / read / add / adopt / remove / install / uninstall / project / update / check / group / tag / library / local / npx / claude`，`--json` 输出字段与 Rust 版一致。内置 `kitter` 技能依赖 CLI，所以 CLI 是对外契约，用 Rust 版输出做快照测试比对。

## 6. UI 层

### 6.1 状态模型

MyGo 是「状态 → `view(c)` 函数」的即时模式写法，没有 GPUI 的 `Entity`/`cx.notify()`：

- 一个根结构 `AppModel`，按 Rust `ui/state.rs` 拆成嵌套子结构：`Shell`、`Skills`、`Projects`、`AddFlow`、`InstallFlow`、`DeleteFlow`、`TagsFlow`、`GroupsFlow`。
- `Entity<InputState>` → 普通 `string` 字段，直接 `ui.TextInput(c, &m.skills.search)`。
- `Entity<SelectState>` → `ui.Select` 绑定到字段。
- `ResizableState` → `float32` 宽度字段，交给 `ui.Split`，写进 `config.json` 保存。
- `ListState` / `ScrollHandle` → 存在模型里的 `ui.ListState` 等组件状态。
- 后台任务（扫描、导入、安装、更新检查）：`go func(){ … win.Update(func(){ 回填状态 }) }()`；用 generation 计数丢弃过期结果（对应 Rust 的 `scan_generation`、`project_snapshot_tasks`）。

### 6.2 组件映射

| Kitter（GPUI） | MyGo `ui` |
|---|---|
| `div().flex()` 布局 | `ui.Row` / `ui.Column` / `ui.Grid`（flex + grid） |
| `Button` 各种 variant | `ui.Button` / `ui.PrimaryButton`，自定义样式 |
| `Checkbox` | `ui.Checkbox` |
| `Input` | `ui.TextInput` / `ui.SearchField` |
| `Select` | `ui.Select` |
| `Popover` | `ui.Popover` |
| `Tooltip` | `.Tooltip(...)` |
| `ContextMenuExt` / `PopupMenuItem` | `.ContextMenu(func(m *ui.Menu){…})` |
| `h_resizable` / `resizable_panel` | `ui.Split` |
| `uniform_list` / `list` | `ui.List`（虚拟化） |
| 自绘模态（`DialogKind`） | `ui.Modal` / `ui.AlertDialog` |
| 详情页 Tabs（Installs / Content） | `ui.Tabs` 或 `ui.Segmented` |
| 技能/标签/分组拖拽排序 | `List.Reorder` + drag-and-drop |
| 标签树 | `ui.Tree` / `ui.Collapsible` |
| 可选择文本（`selectable_text.rs`） | `.Selectable()` |
| 技能文件内容 | `ui.Text` / `ui.RichText`，等宽字体用 JetBrains Mono |
| notice（toast） | `ui.Toast` |
| `motion.rs` spinner / 进度 | `ui.Spinner` / `ui.Progress` + transitions |
| SVG 图标 / Agent PNG 图标 | `ui.Icon`（embed SVG）/ `ui.Image`（embed PNG） |
| 选择文件夹 | `mygo.Dialog.Open(Directory: true)` |
| Finder 中显示 / 打开链接 | `mygo.Shell.ShowItemInFolder` / `OpenExternal` |
| 深浅色 | `mygo.Theme.IsDark` / `SetSource` / `OnUpdated` |
| 窗口材质 | `Vibrancy` + `TitleBarHiddenInset`，侧栏区域透明 |
| macOS 菜单与 ⌘Q | `mygo` 应用菜单（menus.md） |

### 6.3 页面拆分

| Rust | Go | 内容 |
|---|---|---|
| `ui/mod.rs` | `app.go` + `shell.go` | 根视图、导航、窗口尺寸计算、notice |
| `skills_page.rs` | `skills_page.go` | 分组列表、搜索、标签筛选、多选、详情（Installs / Content） |
| `projects_page.rs` + `effective_view.rs` | `projects_page.go` | 项目列表、Agent 维度的有效技能、插件、上下文估算 |
| `settings_page.rs` | `settings_page.go` | 语言、主题、数据目录等 |
| `add_flow.rs` / `add_actions.rs` / `adoption_list.rs` | `dialogs/add.go` | 四种来源的扫描、勾选、分组、导入/采纳 |
| `install_flow.rs` | `dialogs/install.go` | 目标 Agent 选择、项目/全局安装 |
| `organize_flows.rs` / `tags_groups_actions.rs` | `dialogs/tags.go`、`dialogs/groups.go` | 标签和分组的增删改、拖拽、指派 |
| `*_actions.rs` | 对应页面文件里的方法 | 把 action 改成 `AppModel` 上的普通方法 |
| `theme.rs` / `badges.rs` / `components.rs` | `theme.go` / `widgets/` | 视觉常量和复用块 |

### 6.4 i18n

沿用中英双语。`messages.rs` 里的 (zh, en) 表 + 界面文案统一放到 `i18n.go`，用 `T(key)` 查表；首次启动按 `mygo.App.Locale()` 选语言，之后读 `config.json`。

## 7. 构建与发布

- `native/mygo.json`：`name: "Kitter"`、`identifier: "dev.kitter.app"`（与 `resources/Info.plist` 一致）、`version`（从 `Cargo.toml` 同步）、`icon`。
- 开发：`cd native && go tool mygo dev`。
- 打包：`go tool mygo build -platform darwin/universal` → `Kitter.app` + `.dmg`（ad-hoc 签名，`-skip-notarize`）。
- `justfile` 增加 `go-check`（`go test ./...` + `go tool mygo vet`）、`go-app`、`go-run`；CLI 单独 `go build -o build/kitter ./cmd/kitter`。
- 先不接 MyGo 的自动更新，后面需要再加（`updates.github`）。

## 8. 测试

开发与验证环境是 Linux（无 cargo、无系统字体），因此：

- 核心与 CLI 测试必须在 Linux 上通过；macOS 专属路径（`~/Library/Application Support/Kitter`）通过 `KITTER_HOME` 在测试中绕开，路径函数单独做 `runtime.GOOS` 分支测试。
- 每次提交都要通过 `GOOS=darwin GOARCH=arm64 go build ./...`（MyGo 无 cgo，可交叉编译）。
- 无法运行 Rust 生成兼容 fixture，改为对照 Rust 源码里的 serde 定义和 Rust 测试中的 JSON 手写 fixture。
- `ui.NewTester` 在 Linux 上可运行（按文本查找、点击、输入都可用），但没有字体时文字度量不准：UI 测试只断言状态和文本，不断言像素/尺寸。视觉与交互的最终验证由用户在 Mac 上完成。

- **核心**：搬 Rust 的 ~124 个测试，用表驱动；`t.TempDir()` + `KITTER_HOME` 隔离。
- **兼容**：用 Rust 版生成一份真实数据目录做 fixture，Go 读取后断言结构一致，写回后再让 Rust 版读一遍。
- **CLI**：`--json` 输出与 Rust 版做快照对比。
- **UI**：`ui.NewTester(app.View, w, h)` 无窗口测试——点击、输入、右键菜单、深色模式；覆盖添加流程、安装流程、标签/分组编辑、manual-only 切换。
- 手动验证项：中文排版与输入法、毛玻璃、长列表滚动、冷启动时间和内存（与 GPUI 版对比记录）。

## 9. 里程碑

| 阶段 | 内容 | 完成标准 |
|---|---|---|
| M0 骨架 | `native/` module、mygo.json、窗口 + 标题栏 + 毛玻璃 + 侧栏导航 + 主题 + i18n、PATH 修复 | `mygo dev` 打开空壳三页面 |
| M1 核心基础 | model / config / fslink / agents / tags / library / project + 测试 | 能读出现有库、列技能、装/卸到项目 |
| M2 技能页 | 分组列表、搜索、详情（Installs / Content）、manual-only、删除 | 技能页功能对齐 |
| M3 添加与采纳 | source + adoption 核心，Add 对话框四种来源 | 四种来源都能导入 |
| M4 项目页 | effective 核心（扫描、adapters、catalog、上下文估算），项目页与插件视图 | 项目页数据和 Rust 版一致 |
| M5 整理 | 标签、分组、拖拽排序、批量选择与指派、更新检查与进度 | 全部对话框可用 |
| M6 CLI 与发布 | `cmd/kitter` 全子命令、快照测试、`mygo build` 出 dmg、justfile | dmg 可安装运行，CLI 输出一致 |

M1 之后 M2/M3/M4 的核心与 UI 可以交替推进；`effective`（M4）体量最大，可在 M2 期间并行移植。

## 10. 实现时需要确认的细节

- 枚举的 JSON 形态严格按 serde 属性推导：`InstallTarget` 是 `rename_all = "snake_case"`（`claude_code`、`open_code`…）；`SkillOrigin` 是内部标签 `#[serde(tag = "type", rename_all = "snake_case")]`（`{"type":"npx","repository":…}`），注意 `skip_serializing_if` 和 `default` 字段。
- `ignore` crate 在 `effective/scanner` 里用到的具体规则（是否读全局 gitignore、是否跳过隐藏文件），Go 端按需最小实现。
- MyGo `List.Reorder` 能否满足「跨分组拖动技能」，不行就用通用 drag-and-drop 自己实现落点计算。
- 自绘软阴影是否保留；MyGo 默认窗口阴影够用就不复刻。

## 11. 实施记录（2025）

移植按里程碑完成并逐个 commit 推送到 `feat/mygo-native-macos`：

| 里程碑 | 内容 | 关键 commit |
|---|---|---|
| M0+M1 | 骨架 + 核心包（model/config/fslink/agents/tags/library/project） | `0defcd9` |
| M1 评审修复 | — | `744d693` |
| M2 | 技能页（分组列表、搜索、详情、删除） | `ea6ed3d`，竞态修复 `e463328` |
| M3a | source + adoption 核心与导入 | `77da22f`、`b7db537` |
| serde 修正 | `ReferenceKind` 字面值、严格枚举、SkillRecord 字段序 | `13bfb66` |
| M3b | AddSkill 对话框 + pendingNotice 通知缝 | `539ec85` |
| M4a | effective 核心（scanner、catalog、policies、估算） | `e15b372`、`63fa5cd`、`8c9a9fc` |
| M4b | 项目页 + 安装流程 + 估算缓存 | `666b5c9`、`4fcba86`、`348992d` |
| M4b 评审修复 | 每路径独立扫描任务（修掉批量任务顺序 bug） | `771afa0`、`6ea7ed8` |
| M5 | 标签/分组管理对话框、设置页、更新检查 | `644d681` |
| M5b | 拖拽 UI（tag/group/skill）+ 真实更新检查测试 | `cb9a9ce`、`0e334dc` |
| M6 | `cmd/kitter` 全子命令 + golden 测试 | `8548050` |

### 实际偏离方案的地方

- **`skillfile` 包**：计划中把 SKILL.md 解析放在 `model` 内，实际拆成独立包 `core/skillfile`（解析 + 校验 + 名字合法性），model 只做数据结构。
- **`SkillReference` 放进 model**：原计划属 adoption；因为 library 要持久化 references，为避免循环依赖放到了 `core/model`（ReferenceKind 字面值 `"Link"/"Direct"/"Alias"/"Plugin"`，不是 snake_case）。
- **`post` 队列**：gpui 的 `cx.App` 任意线程投递 → `App.Apply` + 可注入的 `post` 函数；后台 goroutine 只允许纯读，库变更全部回 UI 线程执行。
- **`pendingNotice`**：toast 不能由 goroutine 直接发（mygo vet 禁止捕获 `*ui.Context`），改为 `App.pendingNotice` 由 `View` 消费。
- **per-path 扫描**：`requestProjectSnapshots` 一开始把多个 pending 路径合进一个 goroutine，造成 p1 慢扫描阻塞 p2——Rust 的 `project_snapshot_tasks` 本来就是每路径一个任务，评审时改回并为此写了真正的并发测试。
- **拖拽实现**：mygo 一个元素只能接受一种 drag 类型（`s.accepts` 单值），group 头部要同时收 group 与 skill 拖入，解法是用包装 `Column` 承载 group drop、内层行承载 skill drop（命中沿祖先链走）。落点计算抽成纯函数（`dropHalf`/`tagDropAllowed`/`groupDropAllowed`）+ 表驱动测试；端到端用 `ui.NewTester` 的 `Press/Move/Release`。
- **`-race` 纳入验收**：每次 push 前跑 `go test -race -count=3`，比方案要求更严。
- **CLI 参数解析**：按计划用标准库手写分发（位置 dispatch + 容忍「选项在位置参数之后」的 scanner + `--` 终止 + `--flag=value`），没有引入第三方库；clap 语义够薄，不值得依赖。
- **`env::current_exe`**：Rust 只在 npx fixture 测试里重执行自身，Go 版保留 in-process fake `source.Runner`（cmd/kitter 没有 fixture 子命令，重执行不增加覆盖）。
- **effective 类型加 MarshalJSON**：`AgentKind`/`SkillScope`/`SkillSource`/`EffectiveSkill`/`AgentContextEstimate` 为 `project --json` 补了 serde 形态（snake_case 变体、内部 tag、Option→null）；这些是 M6 新增，不属于持久化文件。
- **测试中的 env**：禁用 `os.Setenv("HOME")`；所有 env 读取走注入缝（`effective.envOr` + `envOverride`、`source.HomeDir`），`t.Setenv` 只用于 `KITTER_HOME`。

### 已知与 Rust 的差异

- mygo 的拖拽没有 gpui 的 hover 手柄和浮动预览实体；整行可拖、跟随指针的副本作为预览，交互等价但观感不同。
- `drop_target` 高亮用行上/下边缘的 accent 边框（Rust 是绝对定位的 2px 线）。
- `project` 项目选择器是普通的项目路径列表（Rust 是 gpui `Select` 下拉），选项相同。
- `List.Reorder` 没有用上——mygo 提供通用 Drag/Drop/DragOver，自建了落点计算，能覆盖 tag 同级排序、group 排序、skill→group 三类。
- `.app` 的 `CFBundleIconFile` 是 `AppIcon.icns`（Rust 打包是 `logo.icns`），并多出 `NSPrincipalClass`、`NSSupportsAutomaticGraphicsSwitching` 两个合理默认键；bundle id `dev.kitter.app`、最低系统 12.0 与 `resources/Info.plist` 一致。
- CLI 的 `install`/`uninstall` 等帮助文本为手工对齐 clap 的排版，逐字相同不保证（选项顺序与措辞一致）。

### 打包在 Linux 上的结果

`cd native && go tool mygo build -platform darwin/arm64 -skip-dmg -skip-notarize -sign -` 产出 `build/darwin-arm64/Kitter.app`（14.1 MB）：`Contents/MacOS/Kitter`（darwin/arm64 可执行）、`Contents/Resources/AppIcon.icns` + `icon.png`、`Info.plist`、`PkgInfo`。必须在 Mac 上完成：代码签名（Developer ID 或 ad-hoc `codesign`）、`.dmg` 制作（`-skip-dmg` 关闭）、公证（`-skip-notarize`）、`just package` 的 DMG 背景图。

### 需要在 Mac 上验收的清单

- 视觉：主题（System/Light/Dark）切换、强调色、毛玻璃侧栏、圆角与阴影、深色模式下的边框/分隔线。
- 中文：界面中文文案、中文字体渲染与输入法（TextInput 里的中文输入/候选框位置）。
- 拖拽手感：tag 同级排序、group 排序（管理对话框与技能页列表）、skill 拖到 group 头部；before/after 高亮是否清晰。
- 打包：`just go-app` 产出的 .app 能双击运行；`codesign --verify`；公证后的 dmg。
- Finder/Spotlight 启动时的 PATH：`source.FindTool` 的 fallback（Homebrew、`~/.local/bin`、nvm 版本目录）在 GUI 启动（无 shell PATH）下能找到 `npx`/`git`/`claude`。
- CLI：`go-cli` 产出的 `kitter` 与 `kitter --help`，`list --json` 与 Rust 版输出逐字段对比；`references/install-cli.md` 里的安装路径（`~/.local/bin/kitter`）。
- 性能：长技能列表滚动流畅度、冷启动时间与内存（与 GPUI 版对比）。

### Rust 代码清理（本分支）

`src/`、`crates/`、`Cargo.toml`、`Cargo.lock`、`build.rs`、Rust 打包脚本（`dmg-settings.py`、`package-macos-dmg.sh`）、`resources/Info.plist` 与 `resources/skills/` 已从本分支移除；内置 skill 的唯一源在 `native/core/library/builtin/kitter/`（`builtin_test.go` 校验 embed 完整性），图标/字体源在 `native/app/`（icons、fonts）与 `assets/macos/`（macOS 图标工程）。Rust 版本完整保留在 `feat/manual-only` 和 `main`：需要对比行为时用 `git worktree add ../kitter-rust feat/manual-only` 检出并构建。
