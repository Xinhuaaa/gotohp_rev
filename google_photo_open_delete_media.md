# GooglePhotoOpen：删除媒体接口（MoveToTrash）请求总结

本文把 OpenList 的 `GooglePhotoOpen` 驱动里“删除媒体”的**具体请求**抽取成可复用的接口说明，方便你在另一个项目中实现并喂给 AI。

该驱动的“删除” = **移入回收站（Move to Trash / Soft Delete）**，不是永久删除。

---

## 1) 接口概览（HTTP）

### Endpoint

- `POST https://photosdata-pa.googleapis.com/6439526531001121323/17490284929287180316`
- `Content-Type: application/x-protobuf`

### 请求头（最小集合）

- `Authorization: Bearer <bearer_token>`
- `User-Agent: <google photos android ua>`
- `Accept-Language: <lang>`
- `Accept-Encoding: gzip`

OpenList 中的 UA 形如（示例）：

```
com.google.android.apps.photos/49029607 (Linux; U; Android 9; Pixel XL; ...; Cronet/127.0.6510.5) (gzip)
```

### 响应处理

- 仅判断 HTTP 状态码：`2xx` 视为成功。
- 非 `2xx`：读取 body 并作为错误信息返回。
- **不解析响应 protobuf**（当前实现忽略响应体）。

---

## 2) 输入参数（你需要提供什么）

### `dedupKeys`（string 数组，支持批量）

驱动方法签名：`moveToTrash(dedupKeys []string)`

OpenList 删除时传入的是：

- `dedupKey := obj.GetID()`
- 这个 `ID` 在该驱动的列表实现里实际是 `MediaItem.MediaKey`（`fileToObj()` 将 `MediaKey` 映射为 `model.Object.ID`）

也就是说：**删除请求里填的字符串来自 mediaKey**（代码注释称其为 dedup key）。

---

## 3) Protobuf 请求体（字段号/类型/含义）

请求体通过“手写 protobuf tag”的方式构造（`drivers/google_photo_open/util.go:282`），可抽象为一个“MoveToTrashRequest”消息，核心字段如下：

| Field No. | Wire Type | 语义 | 值/结构 |
|---:|---|---|---|
| 2 | varint | operation type | `1`（move to trash） |
| 3 | len-delimited string (repeated) | item keys | 每个待删除 key 写一次 |
| 4 | varint | operation mode | `1` |
| 8 | len-delimited message | metadata | 固定嵌套结构（主要是空 message） |
| 9 | len-delimited message | client info | 客户端版本信息 |

### Field 8：固定嵌套结构（按代码构造顺序）

等价结构（用路径表示）：

- `8.4.2 = {}`（空 message）
- `8.4.3.1 = {}`（空 message）
- `8.4.4 = {}`（空 message）
- `8.4.5.1 = {}`（空 message）

即 `8` 里只有一个 `4` 子消息，而 `4` 子消息里包含 2/3/4/5 等字段（其中 3 和 5 下面再各有一个 “field 1 empty” 的子消息）。

### Field 9：客户端版本信息

等价结构：

- `9.1 = 5`
- `9.2.1 = <client_version_code>`（OpenList 默认 `49029607`）
- `9.2.2 = "<android_api_version>"`（OpenList 默认 `"28"`，注意是字符串）

---

## 4) 伪 `.proto`（用于喂给 AI 的“结构化描述”）

下面是“按字段号”近似的结构化表达（不是官方 proto，仅用于描述请求体形状）：

```proto
message MoveToTrashRequest {
  int32 operation_type = 2;          // = 1
  repeated string item_key = 3;      // mediaKey strings
  int32 operation_mode = 4;          // = 1

  message Field8 {
    message Field4 {
      message Empty {}
      message Field3 { Empty field1 = 1; }
      message Field5 { Empty field1 = 1; }
      Empty field2 = 2;
      Field3 field3 = 3;
      Empty field4 = 4;
      Field5 field5 = 5;
    }
    Field4 field4 = 4;
  }
  Field8 meta = 8;

  message ClientInfo {
    int32 field1 = 1; // = 5
    message Version {
      int64 client_version_code = 1;
      string android_api_version = 2; // "28"
    }
    Version version = 2;
  }
  ClientInfo client = 9;
}
```

---

## 5) Bearer Token 获取（Android Auth）

删除接口依赖 `Authorization: Bearer ...`，token 由 Android 风格的 auth 接口获取：

### Endpoint

- `POST https://android.googleapis.com/auth`
- `Content-Type: application/x-www-form-urlencoded`

### 表单字段（OpenList 使用的集合）

| key | 来源 |
|---|---|
| `androidId` | `AuthData` 解析 |
| `Email` | `AuthData` 解析 |
| `Token` | `AuthData` 解析（Master token / auth token） |
| `client_sig` | `AuthData` 解析 |
| `lang` | `AuthData` 解析 |
| `callerSig` | `AuthData.callerSig` 或默认等于 `client_sig` |
| `device_country` | 默认 `us` |
| `google_play_services_version` | 默认 `233613038` |
| `oauth2_foreground` | 默认 `1` |
| `sdk_version` | 默认 `28` |
| `service` | 默认：`oauth2:https://www.googleapis.com/auth/photos https://www.googleapis.com/auth/photoslibrary https://www.googleapis.com/auth/plus.me openid email profile` |
| `app` / `callerPkg` | 固定：`com.google.android.apps.photos` |

响应是文本 `key=value` 行格式；OpenList 取其中：

- `Auth=<bearer_token>`
- `Expiry=<unix seconds>`（OpenList 会用配置覆盖过期时间以减少刷新频率）

---

## 6) 实现提示（最小流程）

1. 用你的认证材料换取 `Bearer`（`POST https://android.googleapis.com/auth`）。
2. 构造上面的 protobuf（字段号必须一致）。
3. `POST` 到 `photosdata-pa` 的 trash endpoint，携带 headers。
4. 只要 `2xx` 就当成功；非 `2xx` 记录 body 以便排错。

---

## 7) 风险与兼容性

- 这是 Google Photos 移动端私有协议（`photosdata-pa.googleapis.com` + protobuf 字段号为逆向结果），**可能随 Google 客户端/服务端版本调整而失效**。
- 当前总结只覆盖“移入回收站”；未覆盖“永久删除/恢复/清空回收站”等动作。
