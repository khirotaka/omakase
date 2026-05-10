{
  _config:: {
    name: 'omakase',
    image: 'ghcr.io/khirotaka/omakase:latest',
    replicas: 1,

    // 非機密の環境変数
    env: {
      POLL_INTERVAL_SEC: '30',
      MAX_ITERATION: '5',
      SANDBOX_TEMPLATE: 'coding-agent-sandbox',
      SANDBOX_NAMESPACE: 'default',
    },

    // 機密情報
    secret: {
      name: 'omakase',
    },

    resources: {
      requests: { cpu: '100m', memory: '512Mi' },
      limits: { cpu: '1', memory: '1024Mi' },
    },
  },
}
