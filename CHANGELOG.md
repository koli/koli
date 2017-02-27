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