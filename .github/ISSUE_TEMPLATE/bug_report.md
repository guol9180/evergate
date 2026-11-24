name: 缺陷报告
description: 报告可复现的问题
labels: ["bug"]
body:
  - type: input
    id: version
    attributes:
      label: EverGate 版本
      description: 管理台右下角或 `evergate --version`
      placeholder: v0.5.5
    validations:
      required: true
  - type: dropdown
    id: install
    attributes:
      label: 安装方式
      options:
        - Windows 安装器
        - Linux tar.gz
        - npm
        - 源码构建
    validations:
      required: true
  - type: textarea
    id: what
    attributes:
      label: 现象
      description: 做了什么、期望什么、实际什么。请附请求日志中的错误类型 / 状态码 / 重试轨迹截图（不要包含任何 key 或令牌）
    validations:
      required: true
  - type: textarea
    id: steps
    attributes:
      label: 复现步骤
      placeholder: |
        1. 渠道类型 / 上游供应商 / 协议入口（chat / responses / messages / gemini）
        2. 客户端（Codex / Claude Code / Cline / curl…）
        3. 请求是否流式
    validations:
      required: true
  - type: textarea
    id: logs
    attributes:
      label: 运行日志
      description: 数据目录 logs/evergate.log 中相关片段（脱敏后）
  - type: input
    id: os
    attributes:
      label: 操作系统 / 客户端版本
