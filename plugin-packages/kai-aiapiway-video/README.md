# AI API WAY 视频

保留 KAI影 的 aiapiway-video 渠道 ID。POST /v1/video/generations 创建，GET /v1/video/generations/{taskId} 查询，Bearer 鉴权。顶层仅发送 model、prompt；metadata 包含 duration（1–15 秒）、ratio、resolution、generate_audio、watermark 与 content。content 保留首尾帧/参考图角色，视频和音频分别为 reference_video、reference_audio。最多 9 张图、3 段视频、3 段音频，音频必须同时有图片或视频。参考图片可使用 data URL；视频和音频需要公网 URL。4K 仅允许 filter-off 模型。SUCCESS/SUCCEEDED/COMPLETED 为成功，FAILURE/FAILED/CANCELLED 保留上游原因。错误码、鉴权、SSRF、轮询恢复和结果保存均由宿主处理。

协议源码为 manifest.json，分发文件为上一级同名 .yingce-plugin 包；修改后必须同步更新包。
