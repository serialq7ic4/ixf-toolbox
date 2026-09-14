# 有序列表子块设计

## 背景与根因

GitHub issue #2 复现于 wiki 文档「四、用户请求导流到新集群」：Markdown
中四个有序列表项各自紧跟一个 `Plain` 代码块，写入后飞书显示为四个独立的
`1.`。只读检查目标文档发现，四个 `ordered` block 与「5.3 回滚验证」中正常
连续编号的三个 `ordered` block 字段一致；差异只有前者被同级 `code` block
隔开。飞书通过同一父节点下相邻的 `ordered` block 自动续号，当前 ixf 将步骤
和其代码块都写成根节点的同级 children，因此破坏了列表连续性。

当前读取端还会按父节点全局累加编号，所以会把这些独立列表导出成连续的
`1. 2. 3. 4.`，形成读写不对称。本设计修复写入结构，并同步修复读取端的
嵌套渲染。

## 目标

- 让有序列表项与其紧随的 fenced code（含解析为 Mermaid 图片的代码块）成为同一列表项的直接
  子块，使飞书保持连续编号。
- 让 publish、整体 update、`patch insert` 和 `patch replace-section` 使用
  相同的层级构建逻辑。
- 让远端已有的列表项子块能够被 `docs read` 可靠地导出，写入后再次读取不丢
  代码或图片。
- 保留现有 Mermaid 图片渲染、SVG 优先/PNG 回退、上传和绑定行为。
- 保持普通文本、表格、引用、标题等现有语义，不扩大自动嵌套范围。

## 非目标

- 不增加飞书有序列表的显式起始编号字段；远端 API 当前未发现此类字段。
- 不把普通段落、表格、引用或任意未知块自动移动到列表项下。
- 不改变 `docs update`、`docs patch` 的安全确认、复杂块检查、幂等性和验证
  契约。
- 不引入 Python、浏览器自动化或新的 Mermaid 渲染器。

## 数据模型

在 `docspublish.Spec` 增加通用字段：

```go
Children []Spec
```

`Children` 表示该 Spec 的直接子块，顺序与 Markdown 顺序一致。只有拥有可
容纳 children 的块才由 builder 写入子块；本次实现实际产生 children 的来源
是 `ordered`。每个子 Spec 仍保留自己的 `Kind`、`SourceKind`、`Text`、富文本
runs、表格数据和图片来源信息，不复制或重编码这些字段。

构建后的远端关系必须满足：

```text
page.children = [ordered_1, ordered_2, ordered_3, ordered_4]
ordered_1.children = [code_1]
ordered_2.children = [code_2]
ordered_3.children = [code_3]
ordered_4.children = [code_4]
```

子块的 `parent_id` 指向所属 `ordered` block；子块对象与顶层对象一样进入
`change_map`，图片子块先写入占位对象，再通过现有 `docx_image` 上传/绑定
流程完成 token 绑定。

## Markdown 解析规则

解析器继续忽略空行，但保留块顺序。遇到有序列表项时，先解析该项文本，随后
只把满足以下条件的连续块收为该项的 `Children`：

1. 当前块是 fenced code（包括由 Mermaid fence 生成的图片 Spec）；
2. 当前块直接位于该列表项之后，中间只有空行；
3. 当前块结束后，下一块仍是 fenced code 或 Mermaid image 时继续收集；遇到普通段落、标题、
   bullet、ordered、表格、blockquote 或文件结束则停止。

归属判断以一个显式递增的有序列表组为单位。解析器保留源行编号用于判断：
只有 `1.` 后续为 `2.`、再后续为 `3.` 这类递增关系时，才把两项之间的
fenced blocks 归入前一项；一旦关系中断，候选 fenced blocks 保持顶层。已确认
属于该组的最后一项可接收其后紧随的 fenced blocks。单独一个 ordered item
后接代码块不触发自动归属，避免改变不明确的旧输入。

因此以下输入会形成两个列表项，每项一个 code child：

````markdown
1. 第一步

```Plain
command-one
```

2. 第二步

```Plain
command-two
```
````

列表项之间的 `ordered` Spec 仍是同一层的相邻根级 Spec；代码块不会作为根级
Spec 输出。未带语言标记但首行符合 Mermaid 图类型的 fenced block，仍按现有
规则生成 `image`/`mermaid` Spec，并按相同规则成为 child。

为避免误改变现有文档，以下内容不会自动成为列表项 child：普通段落、表格、
引用、标题、bullet、另一个 ordered item，以及代码块之前已经出现的非空块。
嵌套列表语法也不在本次范围内；现有列表解析行为保持不变。

## 写入构建

将当前只处理顶层 Spec 的 `buildBlocks` 拆为可递归的 block 构建过程：

- 顶层调用以 page token 为 parent，返回顶层 block ID 和全部扁平化
  `blockEntry`。
- 对每个 `ordered` Spec 先创建 ordered block，再以该 block ID 递归构建
  `Children`，把生成的 child IDs 写入 ordered 的 `children` 数组。
- 普通 quote/callout/table 等已有复合块继续使用其现有专用 builder；本次
  parser 不将它们作为 ordered child 自动生成。递归 builder 对非 ordered
  Spec 的非空 `Children` 直接报错，避免静默生成未定义结构。
- 图片序号在整棵 Spec 树上按文档顺序全局递增，确保顶层和子级 Mermaid/本地
  图片的名称不冲突。
- 所有生成的 entry（包括递归子 entry）进入同一个 change map；根节点只插入
  顶层 IDs，父块对象内的 `children` 负责建立子块关系。

`buildBlocks` 的返回签名保持不变，避免 publish/update/patch 的调用方改变；
内部新增递归 helper 和必要的 parent-aware factory 方法。

所有面向 Spec 的统计和检查必须遍历整棵树，而不是只遍历顶层，包括
`summarizeSpecs`、Mermaid renderer readiness、图片/引用/粗体计数、patch
fingerprint 和默认 required text。`plannedTopLevelBlocks` 只统计顶层 Spec，
`plannedBlockEntries` 统计递归生成的全部 block entry。

## 读取与 round-trip

读取端的 ordered 分支改为：

1. 按同一父节点的 children 顺序计算编号；只有相邻的 ordered siblings 继续
   累加，遇到其他 kind 时重置该连续组；每个 parent 独立计数；
2. 渲染有序项文本；
3. 递归渲染其 children，子块使用比列表项更深一级的 Markdown 缩进；
4. 将文本与 children 用稳定的空行连接。

为让读取结果可以再次被解析，reader 输出的 child fenced code/image 使用与
列表项一致的两空格缩进；parser 在识别 fenced code 时接受该层级缩进，并在
识别 ordered/bullet/heading 时保留既有顶层规则。输出示例：

````markdown
1. 第一步

  ```Plain
  command-one
  ```

2. 第二步

  ```Plain
  command-two
  ```
````

对历史上已经被飞书拆成同级 `ordered, code, ordered, code` 的文档，读取端不
再跨非 ordered sibling 合并编号，而是按飞书实际结构分别输出 `1.`；这消除
了当前虚假的连续编号。对相邻 ordered siblings，仍输出连续编号。

## Patch 与验证

`buildPatchInsertChangeMap`、`buildReplaceSectionChangeMap` 和整体替换使用
递归 builder 产生的 entries，不新增另一套 patch 专用结构。现有 root children
操作只处理顶层 IDs，子块关系由新 block 数据中的 `children` 表达，因此：

- `patch insert` 的既有块不产生修改操作；
- section replace 仍只替换章节根级 children；
- 删除旧章节时，已有递归删除逻辑继续覆盖旧子树；
- Mermaid child 的 placeholder、上传、binding 和回读流程与顶层图片相同。

apply 后的验证继续检查 required text、图片数量、代码内容和未改动块。新增
结构断言：回读 graph 中所有预期的 ordered parent-child 边均存在，child 的
kind 和 `parent_id` 与 Spec 树一致；所有生成图片 child 均已绑定有效 image
token。验证输出增加 `expectedNestedBlockCount`、`nestedBlockCount` 和
`missingNestedBlockCount`，验证失败时即使 HTTP 写入成功也返回 `ok:false`。

## 测试策略

先写失败测试，再实现：

- parser：解析“ordered + code + ordered + code”时得到两个 ordered Spec，且
  每个只有对应 code child；连续多个 code child 全部归属前一项；遇到普通段落
  后停止归属；Mermaid fenced block 归属为 image child。
- builder/change map：ordered 顶层 IDs 相邻，ordered 数据含 child IDs，child
  数据的 `parent_id` 正确，所有 child entries 都出现在 change map，且不产生
  额外根级插入操作。
- reader：相邻 ordered siblings 输出连续编号；被 code 隔开的历史同级
  ordered 输出独立 `1.`；ordered child code/image 被缩进输出且内容不丢失。
- publish/update/patch fixture：验证递归 entries、图片 child 上传绑定、
  nested-block verification、`verify.ok` 和既有块不变检查。
- 全量 `go test ./...`、`go vet ./...`、构建及 release smoke 继续使用 Go
  工具链。

## 兼容性与发布

这是针对现有行为的 bug fix，不改变公开命令参数，版本按项目约定从
`3.27.3` 递增为 `3.27.4`，只更新最后一位。发布前更新 CHANGELOG、Go 二进制
版本元数据及 Codex/Claude plugin runtime metadata，并通过
`127.0.0.1:7890` 代理执行 GitHub 操作。
