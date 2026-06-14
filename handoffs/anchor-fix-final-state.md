# 锚点跳底 Bug 最终状态(2026-06-14)

## 结论

**用户接受现状,停止优化。** 分页加载功能可用,焦点基本保持;残留轻微跳动/偶尔空白是架构性限制,完全消除需大重构,不值得继续投入。

## 已解决的问题

- ❌ → ✅ 分页加载时焦点直接跳到最底部(根因:prepend 检测因 group id 改变而失败)
- ❌ → ✅ 加载后焦点随机定位(根因:多层防御抢 scrollTop)
- ❌ → ✅ 加载过程多次跳动(根因:每次 prepend 各触发一次 scrollToIndex)
- ❌ → ✅ 大段空白频繁出现(defaultItemHeight + minOverscanItemCount 压制)

## 残留限制(已知,接受)

- 快速上滑触发翻页时,仍可能出现轻微跳动(Virtuoso prepend 未测量 item 的 probe 渲染)
- 偶尔出现短暂空白(increaseViewportBy/minOverscanItemCount 已大幅减少,但极端快速滚动时仍可能)
- **根因**:react-virtuoso + 动态高度 + iOS WKWebView 的架构限制。Virtuoso 的 UpwardScrollingCompensation 依赖测量时序,iOS WKWebView 上 programmatic scroll 与测量有同步问题
- **完全消除需**:native 拥有滚动状态(prepend 时 native 记住 UIScrollView offset,Web 只渲染),类似 Telegram 的 UICollectionView 方案。工作量:数天重构,另起计划

## 最终生效的机制

### native 侧(cccode-ios)
1. **显式 prepend 信号**:`ChatViewModel.mergeOlderMessagePage` 设置 `pendingPrependAnchorMessageID = messages.first?.id`(prepend 前旧首项的稳定 message id)。render 时随 `WebTimelineSnapshot.prependAnchorMessageID` 传给 Web
2. **grouping 不变式**:`ChatTimelineAdapterUIKit.groupedMessages` 加 `prependBoundaryMessageID` 参数。遇到该 message id 的 assistant,**强制开启新 group**,绝不合并进前面的 assistant 组。保证旧首项保持独立 Virtuoso item,新内容表现为真正新增的前置 items
3. **WebTimelineItem.messageIDs**:每个 item 携带聚合的所有底层 message id,Web 用 `item.messageIDs.includes(anchorMessageID)` 定位 anchor(group id 会变,但 message id 不变)
4. **DEBUG isInspectable**:`MessageWebContainerView` 在 DEBUG 构建设 `webView.isInspectable = true`(iOS 16.4+),诊断能力

### Web 侧(message-web)
1. **firstItemIndex 递减**:prepend 时 `firstItemIndexRef.current -= anchorIndex`。grouping 不变式保证 anchorIndex>=1
2. **prepend lock**:1.5s 内 `followOutput={false}` + streaming RAF 不执行
3. **defaultItemHeight={160}**:跳过 probe 渲染,减少测量修正跳动
4. **increaseViewportBy.top=2400 + minOverscanItemCount.top=6**:保证视口上方渲染充足,防空白

### 已删除的机制(曾是问题源)
- ~~DOM 层 scroll re-clamp 硬锁~~:和 scrollToIndex 抢 scrollTop 造成高频抖动
- ~~scrollToIndex({index:0}) 主动定位~~:替用户决定看哪里,与"焦点不动"需求冲突
- ~~scrollToIndex anchor 复位~~:测量未完成时硬切造成空白闪烁
- ~~items id 差异猜测 prepend~~:group id 改变时失败(前 4 次失败的根因)

## 关键文件改动(cccode-ios,未提交)

- `message-web/src/components/Timeline.tsx` — Web 侧全部机制
- `message-web/src/types.ts` — prependAnchorMessageID / messageIDs 字段
- `OpenCodeiOS/OpenCodeiOS/App/MessageWeb/MessageWebModels.swift` — WebTimelineSnapshot.prependAnchorMessageID / WebTimelineItem.messageIDs
- `OpenCodeiOS/OpenCodeiOS/App/MessageWeb/MessageWebSnapshotBuilder.swift` — 传递新字段
- `OpenCodeiOS/OpenCodeiOS/App/ChatMessageGroupModels.swift` — ChatTimelineGroup.messageIDs
- `OpenCodeiOS/OpenCodeiOS/App/ChatTimelineAdapterUIKit.swift` — grouping 不变式(prependBoundaryMessageID)
- `OpenCodeiOS/OpenCodeiOS/App/MessageWeb/MessageWebContainerView.swift` — isInspectable + scrollToBottom
- `OpenCodeiOS/OpenCodeiOS/App/ChatUIKitContainerView.swift` — 贯通 prependAnchorMessageID
- `OpenCodeiOS/OpenCodeiOS/ViewModels/ChatViewModel.swift` — pendingPrependAnchorMessageID
- `OpenCodeiOS/OpenCodeiOS/ViewModels/ChatViewModel+MessageSync.swift` — mergeOlderMessagePage 记录 anchor
- `OpenCodeiOS/OpenCodeiOS/ViewModels/ChatViewModel+SessionManagement.swift` — session 切换清 anchor
- `OpenCodeiOS/OpenCodeiOSTests/MessageDeduplicationTests.swift` — 测试
- `OpenCodeiOS/OpenCodeiOS/Resources/MessageWeb/assets/index-*.js` — 构建产物
- `OpenCodeiOS/CCCode.xcodeproj/project.pbxproj` — 自动重生成 group id

## 后续(若要彻底解决)

native 拥有滚动状态方案:
- prepend 时 native 记住 WKWebView scrollView 的 contentOffset
- Web 渲染完成后 native 校正 offset(而非依赖 Virtuoso 补偿)
- 或:放弃 WKWebView 内嵌 React 的虚拟滚动,改用 native UITableView/UICollectionView + 已知高度
- 工作量:数天,需重构渲染边界,另起计划

## 相关文档
- 完整 8+次修复复盘:`handoffs/anchor-jump-debug-history.md`
- 第二意见讨论:`handoffs/anchor-plan-keep-focus-discussion.md`
