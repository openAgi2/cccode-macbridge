# 锚点保持方案讨论:分页加载后焦点不动

## 背景

iOS App(cccode-ios,Swift + WKWebView 内嵌 React),codex 模式长会话,分页加载更早消息(prepend)时焦点跳底。历经 8 次修复迭代,当前状态:

- **跳底问题已基本解决**(从"直接跳到最底部"→"定位到刚加载的最旧消息")
- **多次跳动已解决**(去抖合并:一次翻页只跳一次)
- 剩余:加载后自动滚到最旧消息(items[0]),用户希望改成**焦点完全不动,自己往上划看新内容**

## 数据流架构

```
ChatViewModel.messages = [...]
  ↓ Combine ($messages observer)
ChatUIKitContainerView.scheduleRender → renderMessageWebTimeline
  ↓ 构造 WebTimelineSnapshot(含 prependAnchorMessageID + items[].messageIDs)
MessageWebContainerView.apply(snapshot:) → WKWebView evaluateJavaScript
  ↓
Web React App.tsx → Timeline 组件
  ↓
Timeline.tsx → <Virtuoso data={items} firstItemIndex={...} followOutput={...}>
```

## 当前生效的机制(Timeline.tsx)

1. **native 显式 prepend 信号**(第 5 次修复的关键):native 在 `WebTimelineSnapshot` 上标注 `prependAnchorMessageID`(稳定的底层 message id,非 group id),Web 据此判定 prepend。每个 `WebTimelineItem` 带 `messageIDs` 数组,Web 用 `item.messageIDs.includes(anchorMessageID)` 定位 anchor。**不再用 items id 差异猜测**——因为 `groupedMessages` 把连续 assistant 合并成一组,用组首 message id 作 group id,prepend 更早的 assistant 时 group id 会改变,差异猜测失败(这是前 4 次失败的根因)。

2. **firstItemIndex 递减**:prepend 时 `firstItemIndexRef.current -= anchorIndex`,告诉 Virtuoso 有 N 条新 item 加在前面。这是 Virtuoso 保持滚动锚点的原生机制。

3. **prepend lock**:1.5s 内 `followOutput={false}` + streaming RAF `pinToStreamingBottom` 不执行,防止追加内容把视图拉走。

4. **去抖 scrollToIndex(0)**(第 8 次修复):prepend 后 400ms 内无新 prepend,才 `scrollToIndex({index:0, align:'start'})` 滚到最旧消息。一次翻页加载多页只跳一次。

## 8 次修复的关键教训(避免重复)

- **前 4 次失败根因**:prepend 检测靠 items id 差异,group id 因连续 assistant 合并而改变 → `isPrepend=false` → 所有防御代码不执行。第 5 次 native 传显式信号才解决。
- **第 4 次失败的反常点**:DOM 层 onScroll re-clamp 硬锁(`scrollTop 偏离>8px 就强制写回`)都没拦住跳底——因为防御代码根本没触发(prepend 检测失败)。
- **第 5-6 次的抖动根因**:三层防御(DOM re-clamp、scrollToIndex、Virtuoso firstItemIndex 补偿)在同一时间窗抢 scrollTop,互相覆盖 → 高频上下跳动。删掉 DOM re-clamp、改用 firstItemIndex 为主才消除抖动。
- **Virtuoso 4.18.5 的 iOS 补偿**:`react-virtuoso/dist/index.mjs:2038` 有 `UpwardScrollingCompensation`,iOS WKWebView 上用 `scrollBy({top:-f})` 抵消 prepend 高度。**这条路径理论正确,之前失败只是因为 firstItemIndex 没递减(检测失败)**。

## 第二意见:不变式漏洞(GPT 5.5 二次诊断)

如果新加载的 assistant 消息与原首个 assistant group 合并:
- anchor 仍位于 `items[0].messageIDs`;
- `anchorIndex === 0`;
- `firstItemIndex` 不变;
- 但 `items[0]` 的顶部增加了大量内容。

Virtuoso 看到的不是"前面新增 N 个 item",而是"第一个 item 变高"。`firstItemIndex` 无法保持 item 内部的 message anchor。

**彻底修复**:禁止分页 prepend 改写原首个 Virtuoso item。分页边界处不要把新旧两页的连续 assistant 合并,让旧首项继续作为独立 Virtuoso item,新内容表现为真正新增的前置 items。此时:
```ts
newFirstItemIndex = oldFirstItemIndex - prependedPresentationItemCount
```

## 关键文件 / 版本

- Web:`message-web/src/components/Timeline.tsx`(react-virtuoso@4.18.5)
- native 渲染:`OpenCodeiOS/OpenCodeiOS/App/ChatUIKitContainerView.swift`
- WKWebView 容器:`OpenCodeiOS/OpenCodeiOS/App/MessageWeb/MessageWebContainerView.swift`
- snapshot 数据模型:`OpenCodeiOS/OpenCodeiOS/App/MessageWeb/MessageWebModels.swift`
- grouping:`OpenCodeiOS/OpenCodeiOS/App/ChatTimelineAdapterUIKit.swift`
- 完整 8 次修复复盘:`/Users/jacklee/Projects/cccode-macbridge/handoffs/anchor-jump-debug-history.md`
