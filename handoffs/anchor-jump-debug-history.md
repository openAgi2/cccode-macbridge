# 锚点跳底 Bug 修复路径复盘(4 次失败)— 征求第二意见

> 生成时间:2026-06-14。本文档用于让另一个 agent 评估根因,复制内容即可。

## 一、现象

iOS App(cccode-ios 仓库,CCCodeBridge 架构),**codex 模式**长会话:
- 用户向上滚动到消息列表最顶部 → 触发向后分页加载更早的消息(prepend 旧消息)
- **焦点直接跳到最底部**(应停留在用户当时阅读的位置)

已连续 4 次修复尝试,**真机复测均失败**(用户每次反馈"依然跳底")。

## 二、架构(数据流,定位根因的关键)

渲染是 **native Swift + WKWebView 内嵌 React**:

```
ChatViewModel.messages = [...]                      ← 数据源
  ↓ Combine ($messages observer)
ChatUIKitContainerView.scheduleRender(...)          ← native 调度渲染
  ↓
renderMessageWebTimeline(snapshot)                  ← 构造 WebTimelineSnapshot
  ↓
MessageWebContainerView.apply(snapshot:)            ← WKWebView
  ↓ evaluateJavaScript: 发 4 条命令给 Web
    setTheme / setBottomInset / setTypingState / replaceSnapshot
  ↓ Web (React App.tsx)
App.tsx: setSnapshot → Timeline 组件
  ↓
Timeline.tsx → <Virtuoso data={items} firstItemIndex={...}>
```

**关键点**:每次 snapshot apply,native 都会发 `setTypingState(isTyping)`(即使值没变)。Web 侧 `setIsTyping` 是独立 React state,会触发额外渲染。

## 三、四次修复路径

### 修复 1:native bypass(失败)
**改的文件**:`ChatViewModel.swift`、`ChatViewModel+MessageSync.swift`、`ChatViewModel+SessionManagement.swift`、`ChatUIKitContainerView.swift`

**做法**:
- `ChatViewModel` 加 `var pendingOlderPagePrepend = false`
- `mergeOlderMessagePage` 在 `messages=` 赋值前置 `pendingOlderPagePrepend = true`
- `$messages` observer 检测到 prepend → 调 `scheduleRender(bypassBottomStick: true)`
- `scheduleRender` 在 bypass 时清 `pendingRenderForceScroll = false`,传 `preservesScrollAnchor: true`
- `renderMessageWebTimeline(preservesScrollAnchor:)` 在 prepend 时清 `pendingMessageWebScrollToBottom = false`
- `.snapshotApplied` 回调只在 `pendingMessageWebScrollToBottom == true` 时才调 `scrollToBottom`

**堵的路径**:native 侧 `.snapshotApplied → scrollToBottom`、native 累积的 `pendingRenderForceScroll`、`bottomScrollRequest`。

**结果**:真机仍跳底。

### 修复 2:Web prepend lock(失败)
**改的文件**:`Timeline.tsx`

**做法**:
- 加 `prependLockedRef`,检测 `firstItemIndex` 递减(prepend)时设 1500ms 锁
- `pinToStreamingBottom`(streaming RAF 滚底)加 `prependLockedRef.current` 检查,锁期内 return
- `followOutput={isTyping && !prependLockedRef.current ? 'auto' : false}`,锁期内强制 false

**堵的路径**:Web 侧 `followOutput='auto'` 自动跟随底部、streaming RAF 的 `pinToStreamingBottom`。

**结果**:真机仍跳底。

### 修复 3:scrollToIndex 主动复位(失败)
**改的文件**:`Timeline.tsx`

**做法**:
- 不再依赖 Virtuoso 的 `firstItemIndex` 内部补偿(读过 Virtuoso 4.18.5 源码,它在 iOS WKWebView 上有专门的 `UpwardScrollingCompensation` 用 `scrollBy({top:-f})` 抵消 prepend 高度,但依赖尺寸测量时机,不可靠)
- prepend 时记下 anchor(prepend 前第一条消息 ID)
- commit 后用两次 `requestAnimationFrame` 等 Virtuoso 完成新尺寸测量
- 调 `virtuosoRef.scrollToIndex({ index: anchorIndex, align: 'start', behavior: 'auto' })` 把视口精确对齐回 anchor 顶部

**结果**:真机仍跳底。说明** scrollToIndex 复位之后,又被某个机制滚到底了**。

### 修复 4:DOM 层 scroll 硬锁(失败 ⚠️ 最关键)
**改的文件**:`Timeline.tsx`

**做法**:
- 加 `ANCHOR_SCROLL_RECLAMP_PX = 8`
- `VerticalOnlyScroller` 的 `onScroll` 里(`context.onScrollerScroll` → `handleScrollerScroll`),prepend 锁激活且已设定 guard 时:`if (Math.abs(scrollTop - guard) > 8) { scroller.scrollTop = guard; return; }`
- guard 由 scrollToIndex 复位后立即读取 `scroller.scrollTop` 设定
- 这是 **DOM 原生层的滚动拦截**,理论上 native 注入 JS、Virtuoso 内部 scrollBy、followOutput 都绕不过 scroll 事件

**结果**:**真机仍直接跳底**。

⚠️ **这是最反常的一点**:DOM 层 onScroll 拦截 + 强制写回 scrollTop,几乎不可能被绕过。如果它都失败,根因**极可能不在 Web 滚动层**。

## 四、已排除的路径(不要再试)

1. native `.snapshotApplied` 的 `scrollToBottom`(修复 1 已 bypass,prepend 时清零 pending)
2. Web `followOutput='auto'` + streaming RAF `pinToStreamingBottom`(修复 2 已 prepend lock)
3. native 其他 `scrollToBottom` 调用点:`didTapScrollToBottomButton` 仅按钮触发;`performAutomationAction` 仅自动化测试
4. `bottomScrollRequest`:`historyRestore` 分支只在 `messageWebNearBottom` 时 force,用户在顶部时 nearBottom=false
5. `isGenerating`/`isTyping` 误触发:`loadOlderMessages` 开头 `guard !isGenerating`,用户能成功翻页说明 isGenerating=false
6. Codex external-turn probe 的 `replaceMessagesFromServer`:codex 走 `codexExternalTurnProbe` 模式,`serverOnlyCompare=true`,用安全的 `mergeLatestServerPage` 而非 replace
7. 修复 3 的 scrollToIndex:已部署(bundle 含 `scrollToIndex({index,align:"start",behavior:"auto"})`)

## 五、修复 4 失败后形成的核心怀疑(请重点评估)

修复 4 是 DOM 层硬锁,失败说明**防御代码可能根本没执行**。可能性:

**A. prepend 检测失败,防御代码从没触发** ⭐ 最高嫌疑
检测条件:
```ts
const isPrepend = previousSessionIDRef.current === snapshot.sessionID
  && previousFirstItemIDRef.current !== null
  && snapshot.items[0]?.id !== previousFirstItemIDRef.current
  && snapshot.items.some((item) => item.id === previousFirstItemIDRef.current);
```
如果 prepend 后 `snapshot.items[0].id` 恰好等于 `previousFirstItemIDRef.current`(ID 不稳定,或 native 重新生成了稳定 ID 导致首项 ID 没变),或旧首项 ID 在新列表里找不到(dedup 改了 ID),则 `isPrepend=false`,**四层防御全部不触发**。

⚠️ 项目历史里有"stable message ID 反复出 bug"的记录(exec-plan 里 `review-fix-pagination-stable-message-id` 整个 phase),ID 不稳定是已知顽疾。**这是当前最强嫌疑**:不是滚底机制多,而是我的 prepend 检测前提就错了。

**B. scroller 元素被重建(remount)**
如果 prepend 导致 Virtuoso 卸载/重建 scroller DOM 节点,旧的 `onScroll` guard 不挂在新节点上。可能原因:`key` 不稳定、`data` 引用每 render 都新建、session 切换触发 webview reload。

**C. WKWebView 整页 reload**
native 侧某些路径调了 `messageWebView.resetForReload()` 或触发了重新 navigation,整个 React app 重新挂载,scrollTop 归零 + 默认滚底。

**D. native 在 1.5s 锁窗口之后才滚**
native 有个异步任务(如 probe、延迟渲染)在锁过期后才调 `scrollToBottom`。但 codex probe 走 merge 不走 replace,且 `.snapshotApplied` 已 gate,这条较弱。

**E. native 通过 `evaluateJavaScript` 直接操作 scrollTop**,绕过 Web React 逻辑
`MessageWebContainerView.scrollToBottom` 就是 `evaluateJavaScript` 直接 `scroller.scrollTo({top: scrollHeight})`。如果有未发现的调用点(或 throttle 后的延迟执行),会直接写 DOM。但修复 4 的 onScroll 应该能拦到这种写 —— **除非 A 或 B 发生**。

## 六、关键诊断障碍(影响所有结论)

- **以上 4 次修复的结论全部来自代码阅读,没有运行时数据佐证**。
- Safari Web Inspector 连不上:`MessageWebContainerView` 创建 WKWebView 时**从未设 `isInspectable = true`**(iOS 16.4+ 默认 false),所以 Mac Safari → 开发菜单看不到 CCCode 的 WebKit 页面。
- `/usr/bin/log --device <UDID>` 本机不支持该选项;`idevicesyslog` 抓不到 iOS unified logging 的 NSLog(已验证 90s 28 万行系统日志,0 条 `[ANCHOR v2]` 诊断日志)。
- 结果:**不知道 prepend 时 scrollTop 的真实数值变化曲线,也不知道防御代码是否真的执行了**。

## 七、想请教的判断

1. 修复 4(DOM onScroll 硬锁)失败,是否足以推断根因不在 Web 滚动层?还是 DOM scroll re-clamp 本身有 iOS WKWebView 的已知失效场景(如 programmatic scroll 不冒泡、scroll event 节流、`scrollTop=` 写入被忽略)?
2. 怀疑 A(prepend 检测因 ID 不稳定而失败)是否最值得先验证?最快的验证方式是给 `isPrepend` 分支加一个能被 native 读到的信号(如 `window.__anchorFired`),native apply 后立即 `evaluateJavaScript` 读它。
3. 有没有更可靠、不依赖 Virtuoso 内部机制、不依赖 prepend 检测的锚点保持方案?比如:把滚动状态完全搬到 native UIScrollView,放弃 Virtuoso 的虚拟滚动(代价大但确定);或 prepend 时 native 主动发一条"保持锚点"命令,Web 收到后才 setSnapshot。
4. 是否应该先解决诊断障碍(设 `isInspectable=true`,让 Safari Web Inspector 能连),用观察代替推断?前 4 次都是在盲调。

## 八、关键文件 / 版本

- Web 侧全部 4 次修复:`/Users/jacklee/Projects/cccode-ios/message-web/src/components/Timeline.tsx`
- native 渲染调度:`OpenCodeiOS/OpenCodeiOS/App/ChatUIKitContainerView.swift`
- WKWebView 容器:`OpenCodeiOS/OpenCodeiOS/App/MessageWeb/MessageWebContainerView.swift`
- Virtuoso 版本:`react-virtuoso@4.18.5`
- 消息合并 / 稳定 ID:`OpenCodeiOS/OpenCodeiOS/ViewModels/ChatViewModel+MessageSync.swift`
- 最新已装 bundle:`OpenCodeiOS/OpenCodeiOS/Resources/MessageWeb/assets/index-Ctafvy2z.js`(含全部 4 层防御)
