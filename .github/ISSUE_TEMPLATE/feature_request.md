name: 功能建议
description: 提出新能力或改进
labels: ["enhancement"]
body:
  - type: textarea
    id: problem
    attributes:
      label: 要解决的问题
      description: 谁在什么场景下遇到什么困难
    validations:
      required: true
  - type: textarea
    id: proposal
    attributes:
      label: 建议的做法
    validations:
      required: true
  - type: textarea
    id: alternatives
    attributes:
      label: 已考虑的替代方案
  - type: checkboxes
    id: scope
    attributes:
      label: 范围确认
      options:
        - label: 我已阅读 AGENTS.md 禁止事项，本建议不要求引入 Redis / 消息队列 / 微服务 / 多租户计费等
          required: true
