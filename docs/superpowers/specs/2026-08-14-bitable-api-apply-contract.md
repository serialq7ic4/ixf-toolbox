# Bitable API Apply Contract

## Scope

This contract covers API-first `ixf bitable record create --apply` for text and attachment fields, plus `ixf bitable attach --apply` for existing-record attachment fields. It does not use browser UI automation. Browser/CDP captures are diagnostic evidence only.

## Source Metadata

- Fetch clientvars with `GET /space/api/v1/bitable/<base>/clientvars`.
- Decode `data.oldSchema.gzipSchema` as gzip/base64 JSON.
- Use `oldSchema.data.table.meta.rev` as the table `localRev`.
- Use `oldSchema.data.table.viewMap[viewId].property.records` for current record count.
- Use `oldSchema.data.table.userMap[<current user id>]` only for optional `createdExtraInfo`.

## Record Token

- `POST /space/api/bitable/<base>/add_record/token`
- Request: `{"tableID":"<tableId>"}`
- Response success: top-level `code == 0`.
- The returned `addRecordToken` is short-lived server state used by the web client before commit; the record id in `USER_CHANGES` is still client-generated.

## Upload

- `POST /space/api/box/upload/prepare/`
- Request:

```json
{
  "mount_point": "bitable_image",
  "mount_node_token": "<baseToken>",
  "name": "<fileName>",
  "size": 1234,
  "size_checker": false
}
```

- Response success: top-level `code == 0`; read `data.upload_id`, `data.block_size`, and `data.num_blocks`.
- `POST /space/api/box/stream/upload/merge_block/?upload_id=<uploadId>&mount_point=bitable_image`
- Body: raw chunk bytes.
- Headers:
  - `Content-Type: application/octet-stream`
  - `x-seq-list: <comma-separated chunk indexes>`
  - `x-block-list-checksum: <comma-separated Adler-32 checksums for each chunk>`
  - `x-block-origin-size: <block_size from prepare>`
- Response success: top-level `code == 0` and `data.success_seq_list` includes the sent chunk indexes.
- `POST /space/api/box/upload/finish/`
- Request: `{"upload_id":"<uploadId>","num_blocks":<numBlocks>,"mount_point":"bitable_image","push_open_history_record":0}`
- Response success: top-level `code == 0`; read `data.file_token`.

## Record Mutation

- Transport: HTTP RCE API, not browser UI automation.
- Watch base and table first:
  - `POST /space/api/rce/messages?member_id=<generatedMemberId>`
  - Body type `COLLABROOM`, data type `WATCH`.
- Commit:
  - `POST /space/api/rce/messages?member_id=<generatedMemberId>`
  - Body type `BITABLE_TABLE`, data type `USER_CHANGES`.
- `USER_CHANGES.data`:

```json
{
  "member_id": 12345678901234,
  "user_ticket": "",
  "type": "USER_CHANGES",
  "token": "<tableId>",
  "lang": "zh",
  "localRev": 2,
  "operations": "<gzip/base64 JSON operations>",
  "signature": "<uuid>",
  "content_type": "gzip/base64",
  "route_key": "<baseToken>"
}
```

- Decoded operations shape:
- New record creation uses `AddRecord` / `data.addRecord`:

```json
[
  {
    "command": "AddRecord",
    "type": 2,
    "actions": [
      {
        "action": "data.addRecord",
        "type": 2,
        "tableId": "<tableId>",
        "viewId": "<viewId>",
        "recordId": "<clientGeneratedRecordId>",
        "data": {
          "indexes": {"<viewId>": 0},
          "cellData": {
            "<textFieldId>": {"type": 1, "value": [{"type": "text", "text": "value"}]},
            "<attachmentFieldId>": {
              "type": 17,
              "value": [
                {
                  "id": "<fileToken>",
                  "attachmentToken": "<fileToken>",
                  "name": "file.png",
                  "mimeType": "image/png",
                  "size": 1234,
                  "timeStamp": 1786718860252
                }
              ]
            }
          },
          "createdExtraInfo": {"name": "", "enName": "", "avatarUrl": ""},
          "total": 34
        }
      }
    ],
    "syncFlag": 0
  }
]
```

- Existing-record attachment append uses `SetRecord` / `data.setRecord`. The action `data` is a map of field id to cell data; for attachment fields, preserve existing attachment values and append uploaded file values before sending:

```json
[
  {
    "command": "SetRecord",
    "type": 2,
    "actions": [
      {
        "action": "data.setRecord",
        "type": 2,
        "tableId": "<tableId>",
        "viewId": "<viewId>",
        "recordId": "<existingRecordId>",
        "viewType": 1,
        "data": {
          "<attachmentFieldId>": {
            "type": 17,
            "value": [
              {
                "id": "<existingOrNewFileToken>",
                "attachmentToken": "<existingOrNewFileToken>",
                "name": "file.png",
                "mimeType": "image/png",
                "size": 1234,
                "timeStamp": 1786718860252
              }
            ]
          }
        }
      }
    ],
    "syncFlag": 0
  }
]
```

- Response success: top-level `code == 0` and `data.type == "ACCEPT_COMMIT"`.

## Verification

- Refetch `/space/api/v1/bitable/<base>/clientvars`.
- Decode `oldSchema.gzipSchema`.
- Verify expected text values exist in the created or matching record.
- Verify expected attachment file names exist in the attachment field value.
- For existing-record attachment append, verify the same `recordId` still exists and its target attachment field contains the uploaded file name.

## Sanitization

This note intentionally omits cookies, CSRF values, full base tokens, full file tokens, raw binary bodies, private URL query tickets, and raw captured payloads.
