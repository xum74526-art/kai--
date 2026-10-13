# AI API WAY 视频接口

配置 `apiKey` 由宿主注入 Bearer 请求头；插件不保存密钥或改变出站安全规则。

## 创建和查询

使用 `POST /v1/video/generations` 创建，`GET /v1/video/generations/{taskId}` 只查询已提交任务。`model`、`prompt` 放在顶层，`duration`、`aspectRatio`、`resolution`、`generateAudio`、`watermark` 映射到 `metadata`。`images`、`videos`、`audios` 统一映射到 `metadata.content`，保留媒体类型及图片角色。

图片支持 data URL；视频和音频由宿主准备公网 URL。图片最多 9 张，视频和音频各最多 3 段，音频必须搭配图片或视频。4K 仅允许模型名包含 filter-off，fast 模型的 1080p 自动使用 720p。

## 响应

创建响应从 data.task_id、task_id、data.id、id 提取上游任务 ID。查询响应读取 data.status/status、data.result_url/result_url 或 video_url，失败保留 fail_reason、error.message、message。鉴权或错误码失败由宿主返回真实错误；已持久化的上游任务超时后只恢复查询，不重新创建或重复计费。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "kai-aiapiway-video",
  "name": "AI API WAY 视频",
  "version": "1.0.0",
  "author": "KAI影",
  "description": "KAI影现有 AIAPIWay Seedance 视频渠道。",
  "documentation": "<当前插件的完整 documentation，由 README.md 与 docs/interface.md 拼接而成；为避免 JSON 递归，此处不重复展开正文。>",
  "permissions": [
    "generation.run",
    "media.read"
  ],
  "configuration": {
    "fields": [
      {
        "name": "apiKey",
        "type": "secret",
        "label": "API Key",
        "required": true
      }
    ]
  },
  "contributes": {
    "providers": [
      {
        "id": "aiapiway-video",
        "label": "AI API WAY 视频",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "渠道授权的 Seedance 模型标识"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频生成提示词"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "metadata.duration",
            "description": "视频时长，范围 1 至 15 秒"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "metadata.ratio",
            "description": "输出宽高比，默认 16:9"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "metadata.resolution",
            "description": "输出清晰度，4K 仅支持 filter-off 模型"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "metadata.generate_audio",
            "description": "是否生成声音"
          },
          {
            "name": "watermark",
            "type": "boolean",
            "required": false,
            "mapping": "metadata.watermark",
            "description": "是否保留水印"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "metadata.content",
            "description": "最多 9 张参考图片，可内嵌 data URL 并保留首尾帧角色"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "metadata.content",
            "description": "最多 3 段公网参考视频"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "metadata.content",
            "description": "最多 3 段公网参考音频，不能单独使用"
          }
        ],
        "validations": [
          {
            "assert": {
              "$and": [
                {
                  "$gte": [
                    {
                      "$ref": "request.duration"
                    },
                    1
                  ]
                },
                {
                  "$lte": [
                    {
                      "$ref": "request.duration"
                    },
                    15
                  ]
                }
              ]
            },
            "message": "AI API WAY Seedance 视频时长需要在 1–15 秒之间"
          },
          {
            "assert": {
              "$and": [
                {
                  "$lte": [
                    {
                      "$len": {
                        "$ref": "request.images"
                      }
                    },
                    9
                  ]
                },
                {
                  "$lte": [
                    {
                      "$len": {
                        "$ref": "request.videos"
                      }
                    },
                    3
                  ]
                },
                {
                  "$lte": [
                    {
                      "$len": {
                        "$ref": "request.audios"
                      }
                    },
                    3
                  ]
                }
              ]
            },
            "message": "AI API WAY 最多支持 9 张参考图、3 个参考视频和 3 个参考音频"
          },
          {
            "assert": {
              "$or": [
                {
                  "$eq": [
                    {
                      "$len": {
                        "$ref": "request.audios"
                      }
                    },
                    0
                  ]
                },
                {
                  "$gt": [
                    {
                      "$len": {
                        "$ref": "request.images"
                      }
                    },
                    0
                  ]
                },
                {
                  "$gt": [
                    {
                      "$len": {
                        "$ref": "request.videos"
                      }
                    },
                    0
                  ]
                }
              ]
            },
            "message": "AI API WAY 的参考音频不能单独使用，请同时提供参考图片或参考视频"
          },
          {
            "assert": {
              "$or": [
                {
                  "$not": {
                    "$in": [
                      {
                        "$lower": {
                          "$trim": {
                            "$ref": "request.resolution"
                          }
                        }
                      },
                      [
                        "2160",
                        "2160p",
                        "4k"
                      ]
                    ]
                  }
                },
                {
                  "$gt": [
                    {
                      "$len": {
                        "$split": [
                          {
                            "$lower": {
                              "$trim": {
                                "$ref": "request.model"
                              }
                            }
                          },
                          "filter-off"
                        ]
                      }
                    },
                    1
                  ]
                }
              ]
            },
            "message": "AI API WAY 的 4K 输出只支持 filter-off 海外版模型"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/video/generations",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$trim": {
                "$ref": "request.prompt"
              }
            },
            "metadata": {
              "duration": {
                "$ref": "request.duration"
              },
              "ratio": {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$ref": "request.aspectRatio"
                      },
                      [
                        "16:9",
                        "9:16",
                        "1:1",
                        "4:3",
                        "3:4",
                        "21:9"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "request.aspectRatio"
                  },
                  "else": "16:9"
                }
              },
              "resolution": {
                "$switch": {
                  "cases": [
                    {
                      "when": {
                        "$in": [
                          {
                            "$lower": {
                              "$trim": {
                                "$ref": "request.resolution"
                              }
                            }
                          },
                          [
                            "480",
                            "480p",
                            "low"
                          ]
                        ]
                      },
                      "then": "480p"
                    },
                    {
                      "when": {
                        "$in": [
                          {
                            "$lower": {
                              "$trim": {
                                "$ref": "request.resolution"
                              }
                            }
                          },
                          [
                            "1080",
                            "1080p"
                          ]
                        ]
                      },
                      "then": {
                        "$if": {
                          "condition": {
                            "$gt": [
                              {
                                "$len": {
                                  "$split": [
                                    {
                                      "$lower": {
                                        "$trim": {
                                          "$ref": "request.model"
                                        }
                                      }
                                    },
                                    "fast"
                                  ]
                                }
                              },
                              1
                            ]
                          },
                          "then": "720p",
                          "else": "1080p"
                        }
                      }
                    },
                    {
                      "when": {
                        "$in": [
                          {
                            "$lower": {
                              "$trim": {
                                "$ref": "request.resolution"
                              }
                            }
                          },
                          [
                            "2160",
                            "2160p",
                            "4k"
                          ]
                        ]
                      },
                      "then": "4k"
                    }
                  ],
                  "default": "720p"
                }
              },
              "generate_audio": {
                "$ref": "request.generateAudio"
              },
              "watermark": {
                "$ref": "request.watermark"
              },
              "content": {
                "$omitEmpty": {
                  "$concatArrays": [
                    {
                      "$map": {
                        "from": {
                          "$sortByOrder": {
                            "$ref": "request.images"
                          }
                        },
                        "as": "media",
                        "in": {
                          "type": "image_url",
                          "role": {
                            "$coalesce": [
                              {
                                "$ref": "media.role"
                              },
                              "reference_image"
                            ]
                          },
                          "image_url": {
                            "url": {
                              "$coalesce": [
                                {
                                  "$ref": "media.dataUrl"
                                },
                                {
                                  "$ref": "media.url"
                                }
                              ]
                            }
                          }
                        }
                      }
                    },
                    {
                      "$map": {
                        "from": {
                          "$sortByOrder": {
                            "$ref": "request.videos"
                          }
                        },
                        "as": "media",
                        "in": {
                          "type": "video_url",
                          "role": "reference_video",
                          "video_url": {
                            "url": {
                              "$coalesce": [
                                {
                                  "$ref": "media.dataUrl"
                                },
                                {
                                  "$ref": "media.url"
                                }
                              ]
                            }
                          }
                        }
                      }
                    },
                    {
                      "$map": {
                        "from": {
                          "$sortByOrder": {
                            "$ref": "request.audios"
                          }
                        },
                        "as": "media",
                        "in": {
                          "type": "audio_url",
                          "role": "reference_audio",
                          "audio_url": {
                            "url": {
                              "$coalesce": [
                                {
                                  "$ref": "media.dataUrl"
                                },
                                {
                                  "$ref": "media.url"
                                }
                              ]
                            }
                          }
                        }
                      }
                    }
                  ]
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/video/generations/{{taskId}}"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.data.task_id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.data.id"
              },
              {
                "$ref": "response.id"
              },
              {
                "$ref": "taskId"
              }
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.data.status"
              },
              {
                "$ref": "response.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.data.fail_reason"
              },
              {
                "$ref": "response.fail_reason"
              },
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.msg"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.data.result_url"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.video_url"
              }
            ]
          },
          "errorPaths": [
            "code",
            "error.code"
          ],
          "resultEphemeral": true
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
