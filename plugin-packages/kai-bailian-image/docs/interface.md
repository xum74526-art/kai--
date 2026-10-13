# 阿里百炼图片接口

配置 `apiKey` 由宿主注入 Bearer 请求头；插件不保存密钥。配置 Base URL 使用 origin，以兼容 compatible-mode/v1 地址。

## 创建和查询

使用 `POST /api/v1/services/aigc/image-generation/generation`，设置 `X-DashScope-Async: enable`；查询使用 `GET /api/v1/tasks/{taskId}`。`model` 为千问或万相模型，`prompt` 与可选 instructions 合并到消息文本，`images` 作为参考图消息且优先 data URL。`imageCount` 映射为 parameters.n，`aspectRatio` 映射为 parameters.size；支持宽x高、推荐比例以及适用模型的 1K/2K/4K 档位。不支持蒙版、参考视频或音频。

正常生成不覆盖供应商提示词扩写或思考默认值；连接测试才关闭对应功能，以缩短测试耗时。4K 仅支持 wan2.7-image-pro 文生图，具体数量、尺寸、字数限制由宿主能力校验。

## 响应

创建响应读取 output.task_id；查询读取 output.task_status，成功从 output.choices[].message.content[].image 提取 URL 或 data URL。code/output.code 及 message/output.message 保留上游错误。已持久化的上游任务超时后只恢复查询，不重新提交。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "kai-bailian-image",
  "name": "阿里百炼图片",
  "version": "1.0.0",
  "author": "KAI影",
  "description": "KAI影现有万相与千问原生图片渠道。",
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
        "id": "bailian-image",
        "label": "阿里百炼图片",
        "capabilities": [
          "image"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://dashscope.aliyuncs.com",
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
            "description": "百炼千问或万相图片模型标识"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "input.messages",
            "description": "图片生成提示词，可与 instructions 合并"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "input.messages",
            "description": "编辑参考图，优先内嵌 data URL，不支持蒙版"
          },
          {
            "name": "imageCount",
            "type": "integer",
            "required": false,
            "mapping": "parameters.n",
            "description": "生成数量，默认 1，模型能力限定最大数量"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "parameters.size",
            "description": "尺寸档位、推荐宽高比或自定义宽x高"
          }
        ],
        "validations": [
          {
            "assert": {
              "$eq": [
                {
                  "$len": {
                    "$ref": "request.videos"
                  }
                },
                0
              ]
            },
            "message": "阿里百炼图片协议不支持参考视频"
          },
          {
            "assert": {
              "$eq": [
                {
                  "$len": {
                    "$ref": "request.audios"
                  }
                },
                0
              ]
            },
            "message": "阿里百炼图片协议不支持参考音频"
          },
          {
            "assert": {
              "$eq": [
                {
                  "$len": {
                    "$filter": {
                      "from": {
                        "$ref": "request.images"
                      },
                      "as": "media",
                      "where": {
                        "$eq": [
                          {
                            "$ref": "media.role"
                          },
                          "mask"
                        ]
                      }
                    }
                  }
                },
                0
              ]
            },
            "message": "阿里百炼图片协议不支持蒙版文件"
          },
          {
            "assert": {
              "$gt": [
                {
                  "$len": {
                    "$trim": {
                      "$ref": "request.prompt"
                    }
                  }
                },
                0
              ]
            },
            "message": "阿里百炼图片请求缺少提示词"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/api/v1/services/aigc/image-generation/generation",
          "originPath": true,
          "contentType": "application/json",
          "headers": {
            "X-DashScope-Async": "enable"
          },
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "input": {
              "messages": [
                {
                  "role": "user",
                  "content": {
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
                            "image": {
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
                      },
                      [
                        {
                          "text": {
                            "$if": {
                              "condition": {
                                "$ref": "request.instructions"
                              },
                              "then": {
                                "$concat": [
                                  {
                                    "$ref": "request.instructions"
                                  },
                                  "\n\n",
                                  {
                                    "$ref": "request.prompt"
                                  }
                                ]
                              },
                              "else": {
                                "$ref": "request.prompt"
                              }
                            }
                          }
                        }
                      ]
                    ]
                  }
                }
              ]
            },
            "parameters": {
              "n": {
                "$if": {
                  "condition": {
                    "$gt": [
                      {
                        "$ref": "request.imageCount"
                      },
                      0
                    ]
                  },
                  "then": {
                    "$ref": "request.imageCount"
                  },
                  "else": 1
                }
              },
              "watermark": false,
              "size": {
                "$omitEmpty": {
                  "$switch": {
                    "cases": [
                      {
                        "when": {
                          "$in": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            [
                              "1k",
                              "2k",
                              "4k"
                            ]
                          ]
                        },
                        "then": {
                          "$upper": {
                            "$lower": {
                              "$trim": {
                                "$ref": "request.aspectRatio"
                              }
                            }
                          }
                        }
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "1:1"
                          ]
                        },
                        "then": "2048*2048"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "16:9"
                          ]
                        },
                        "then": "2688*1536"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "9:16"
                          ]
                        },
                        "then": "1536*2688"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "4:3"
                          ]
                        },
                        "then": "2368*1728"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "3:4"
                          ]
                        },
                        "then": "1728*2368"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "3:2"
                          ]
                        },
                        "then": "2496*1664"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "2:3"
                          ]
                        },
                        "then": "1664*2496"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$lower": {
                                "$trim": {
                                  "$ref": "request.aspectRatio"
                                }
                              }
                            },
                            "21:9"
                          ]
                        },
                        "then": "3072*1312"
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$len": {
                                "$split": [
                                  {
                                    "$lower": {
                                      "$trim": {
                                        "$ref": "request.aspectRatio"
                                      }
                                    }
                                  },
                                  "x"
                                ]
                              }
                            },
                            2
                          ]
                        },
                        "then": {
                          "$concat": [
                            {
                              "$at": [
                                {
                                  "$split": [
                                    {
                                      "$lower": {
                                        "$trim": {
                                          "$ref": "request.aspectRatio"
                                        }
                                      }
                                    },
                                    "x"
                                  ]
                                },
                                0
                              ]
                            },
                            "*",
                            {
                              "$at": [
                                {
                                  "$split": [
                                    {
                                      "$lower": {
                                        "$trim": {
                                          "$ref": "request.aspectRatio"
                                        }
                                      }
                                    },
                                    "x"
                                  ]
                                },
                                1
                              ]
                            }
                          ]
                        }
                      },
                      {
                        "when": {
                          "$eq": [
                            {
                              "$len": {
                                "$split": [
                                  {
                                    "$lower": {
                                      "$trim": {
                                        "$ref": "request.aspectRatio"
                                      }
                                    }
                                  },
                                  "×"
                                ]
                              }
                            },
                            2
                          ]
                        },
                        "then": {
                          "$concat": [
                            {
                              "$at": [
                                {
                                  "$split": [
                                    {
                                      "$lower": {
                                        "$trim": {
                                          "$ref": "request.aspectRatio"
                                        }
                                      }
                                    },
                                    "×"
                                  ]
                                },
                                0
                              ]
                            },
                            "*",
                            {
                              "$at": [
                                {
                                  "$split": [
                                    {
                                      "$lower": {
                                        "$trim": {
                                          "$ref": "request.aspectRatio"
                                        }
                                      }
                                    },
                                    "×"
                                  ]
                                },
                                1
                              ]
                            }
                          ]
                        }
                      }
                    ],
                    "default": null
                  }
                }
              },
              "prompt_extend": {
                "$if": {
                  "condition": {
                    "$ref": "request.extra.connectionTest"
                  },
                  "then": false,
                  "else": null
                }
              },
              "enable_thinking": {
                "$if": {
                  "condition": {
                    "$and": [
                      {
                        "$ref": "request.extra.connectionTest"
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
                                "qwen-image-3.0"
                              ]
                            }
                          },
                          1
                        ]
                      }
                    ]
                  },
                  "then": false,
                  "else": null
                }
              },
              "enable_interleave": {
                "$if": {
                  "condition": {
                    "$and": [
                      {
                        "$ref": "request.extra.connectionTest"
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
                                "wan2.7-image"
                              ]
                            }
                          },
                          1
                        ]
                      }
                    ]
                  },
                  "then": false,
                  "else": null
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/api/v1/tasks/{{taskId}}",
          "originPath": true
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.output.task_id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "taskId"
              }
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.output.task_status"
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
                "$ref": "response.output.message"
              },
              {
                "$ref": "response.message"
              }
            ]
          },
          "images": {
            "$map": {
              "from": {
                "$concatArrays": [
                  {
                    "$ref": "response.output.choices.0.message.content"
                  },
                  {
                    "$ref": "response.output.choices.1.message.content"
                  },
                  {
                    "$ref": "response.output.choices.2.message.content"
                  },
                  {
                    "$ref": "response.output.choices.3.message.content"
                  },
                  {
                    "$ref": "response.output.choices.4.message.content"
                  },
                  {
                    "$ref": "response.output.choices.5.message.content"
                  }
                ]
              },
              "as": "item",
              "in": {
                "$omitEmpty": {
                  "$ref": "item.image"
                }
              }
            }
          },
          "errorPaths": [
            "code",
            "output.code"
          ],
          "messagePaths": [
            "message",
            "output.message"
          ],
          "resultEphemeral": true
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
