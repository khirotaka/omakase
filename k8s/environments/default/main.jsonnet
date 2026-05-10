local k = import 'k.libsonnet';
local container = k.core.v1.container;
local deployment = k.apps.v1.deployment;
local envFrom = k.core.v1.envFromSource;

(import './config.libsonnet') + {
  local c = $._config,

  deployment:
    deployment.new(c.name, c.replicas, containers=[
      container.new(c.name, image=c.image)
      + container.withImagePullPolicy('Always')
      + container.withEnvMap(c.env)
      + container.withEnvFrom(envFrom.secretRef.withName(c.secret.name))
      + container.withResourcesRequests(cpu=c.resources.requests.cpu, memory=c.resources.requests.memory)
      + container.withResourcesLimits(cpu=c.resources.limits.cpu, memory=c.resources.limits.memory),
    ])
    + deployment.spec.strategy.withType('Recreate'),
}
