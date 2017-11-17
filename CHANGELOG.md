# v0.9.0-alpha

## Features

- Add metadata API for storing releases [#142](https://github.com/koli/koli/issues/142)
- Add default route (service/ingress) to deployment apps [#137](https://github.com/koli/koli/issues/137)
- Support deploys from any git server [#142](https://github.com/koli/koli/issues/142)

# v0.6.2-alpha

## Improvements

### controller

- Remove deprecated namespace validation.

# v0.6.1-alpha

## Bugs

- Implement correctly removing ingresses resources. [#114](https://github.com/kolihub/koli/issues/114)

# v0.6.0-alpha

## Features

- API for mutating kubernetes resources requests for specific endpoints
  - Namespaces
  - Deployments
  - Ingresses (POST, PATCH)
  - Domain (platform)
- Automatic provisoning of Persistent Volumes based on service plans
- Storage Service Plan
- Event notification on error [#48](https://github.com/kolihub/koli/issues/48)

## Improvements

- Merge mutator and gitstep codebases [#109](https://github.com/kolihub/koli/issues/109)
- Prevent listing all namespaces from cluster [#105](https://github.com/kolihub/koli/issues/105)
- Decode RS256 token in GIT Server [#104](https://github.com/kolihub/koli/issues/104)
- Update k8s library to v1.6.0 [#92](https://github.com/kolihub/koli/issues/92)
- Improve queue system [#62](https://github.com/kolihub/koli/issues/62)
- Koli-system namespace must access all services from the cluster [#95](https://github.com/kolihub/koli/issues/95)
- Remove slugbuilds when a deploy is removed [#108](https://github.com/kolihub/koli/issues/108)

## Bugs

- Error when creating/updating hook on gitstep [#93](https://github.com/kolihub/koli/issues/93)
- Dirty slugbuild pods trigger new deploy updates [#96](https://github.com/kolihub/koli/issues/96)
- Orphan 3PR hangs the execution of the controller [#65](https://github.com/kolihub/koli/issues/65)

# v0.5.0-alpha

## Features

- GitHub webhook integration [#77](https://github.com/kolihub/koli/issues/77)
- Centralize upload/download slugs to gitstep [#88](https://github.com/kolihub/koli/issues/88)

## Improvements

- Cleanup slug after build [#87](https://github.com/kolihub/koli/issues/87)

# v0.4.0-alpha

## Features

- Application release concept [#73](https://github.com/kolihub/koli/issues/73)
  - Builder Controller [#74](https://github.com/kolihub/koli/issues/74)
  - Release Controller [#73](https://github.com/kolihub/koli/issues/73)
  - Deployer Controller [#61](https://github.com/kolihub/koli/issues/61)

## Improvements

- Expiration of releases on deploy
- Deploy after building apps
- Remove rebuilding releases, resources must be immutable
- Add build revisions

# v0.3.0-alpha

## Features

- Koli Kubernetes controller for managing third party resources [#36](https://github.com/kolihub/koli/issues/36), [#36](https://github.com/kolihub/koli/issues/38)
- Platform namespace wrapper (organization concept) [#42](https://github.com/kolihub/koli/issues/42)
- Added addons concept as statefulsets resources  [#43](https://github.com/kolihub/koli/issues/43)
- Added Service Plans concept [#54](https://github.com/kolihub/koli/issues/54)
- Allocate compute resources based on service plans [#56](https://github.com/kolihub/koli/issues/56)


## Improvements

- Use KeyFunc on Namespace controller [#55](https://github.com/kolihub/koli/issues/55)
- Associate a default network policy to namespaces [#52](https://github.com/kolihub/koli/issues/52)
- Change creation of RoleBinding [#53](https://github.com/kolihub/koli/issues/53)
- Change makefile to build the controller with version [#49](https://github.com/kolihub/koli/issues/49)
