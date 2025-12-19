
## 2025.12.1 (2025/12/19)

### Documentation

-  Warn that terraform needs HTTPS with a valid certificate. ([e9e93510](https://github.com/elisiariocouto/specular/commit/e9e93510a58b955b5354c0179f636f2635acee4a))
-  Fix quick start discrepancies. ([3391c527](https://github.com/elisiariocouto/specular/commit/3391c5276f5fe9789e47f77c86d238c15935d5b6))


### Miscellaneous Tasks

-  Rename project. ([bc00165d](https://github.com/elisiariocouto/specular/commit/bc00165d3f6258f7ff8b2c7c9cf6dc2999183dbd))



## 2025.12.0 (2025/12/18)

### Bug Fixes

- **mirror:** Cache rewritten version JSON and stream archives to reduce memory, use errors.Is for NotFound, simplify filename extraction. ([58005863](https://github.com/elisiariocouto/specular/commit/58005863108e54e97a237b4fd64965248a0f6ba1))


### Features

- **config:** Improve config management and add tests. ([4c11e0c6](https://github.com/elisiariocouto/specular/commit/4c11e0c6aca60af1949f74cc72aaeff4c49be7c6))
-  Build binaries, container images and add release tooling. ([d1c08b06](https://github.com/elisiariocouto/specular/commit/d1c08b06089382b2e3ece2378dd0e1695b9902de))


### Miscellaneous Tasks

-  Fix tests. ([578421c2](https://github.com/elisiariocouto/specular/commit/578421c2dbb09df31e14505becee399eb3581862))
-  Bump go versions in CI. ([9b4bab04](https://github.com/elisiariocouto/specular/commit/9b4bab04c61d1c6893ce1592932dedff0fb2b447))


### Refactor

- **mirror:** Add request coalescing and URL validation to discovery cache. ([79e31334](https://github.com/elisiariocouto/specular/commit/79e31334d26a8d2da68fab0d197a47dfd55a7516))
- **mirror:** Extract archive URL building and remove version extraction from filename ([b71a641b](https://github.com/elisiariocouto/specular/commit/b71a641bb8d2adaa2289d953eb01a47869f4fd1e))
- **mirror:** Extract helpers for platform key and filename; add cache write logging ([d250f2eb](https://github.com/elisiariocouto/specular/commit/d250f2eb892b82228578eed4d098807c1689f23c))
- **upstream:** Improve error handling, add URL validation, and configurable cache TTL. ([f995da1b](https://github.com/elisiariocouto/specular/commit/f995da1b15dfc8c94a3fd6b9856401d0435a67a4))
-  Fix lint issues and improve code quality. ([5a2a564e](https://github.com/elisiariocouto/specular/commit/5a2a564eb28281feef1002e64591ecc74f71ae06))
-  Add type-safe validation and comprehensive tests for mirror types. ([8ac7e0d0](https://github.com/elisiariocouto/specular/commit/8ac7e0d0d484c3da8a4eeebd1d4400ab5fe5dbfc))


# Changelog

All notable changes to this project will be documented in this file.

This project adheres to [Calendar Versioning](https://calver.org/).
