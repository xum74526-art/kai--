# 阿里百炼图片

保留 KAI影 的 bailian-image 渠道 ID，使用百炼原生异步接口。创建 POST /api/v1/services/aigc/image-generation/generation，带 X-DashScope-Async: enable；查询 GET /api/v1/tasks/{taskId}。Bearer 鉴权，使用所配置 Base URL 的 origin，兼容 compatible-mode/v1 地址。input.messages[].content 同时包含提示词与参考图；图优先 data URL。不支持蒙版、视频或音频输入。parameters.n 控制张数；size 保留既有 1K/2K/4K 档位、比例推荐尺寸及自定义宽x高到宽*高转换。4K 仅允许 wan2.7-image-pro 文生图。正式生成保留供应商提示词扩写/思考默认值，连接测试关闭对应模型的扩写/思考。结果读取 output.choices[].message.content[].image，失败保留 code/message。异步恢复、重试、权限与结果下载使用宿主统一流程。

协议源码为 manifest.json，分发文件为上一级同名 .yingce-plugin 包；修改后必须同步更新包。
