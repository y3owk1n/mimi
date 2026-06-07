# Changelog

## [0.5.0](https://github.com/y3owk1n/mimi/compare/v0.4.0...v0.5.0) (2026-06-07)


### Features

* add IPC subsystem and event-driven workspace title ([#27](https://github.com/y3owk1n/mimi/issues/27)) ([fdd209b](https://github.com/y3owk1n/mimi/commit/fdd209b847defb39cbc948edf3c64bda4275a608))
* changes scope of this project to space and window utility ([#25](https://github.com/y3owk1n/mimi/issues/25)) ([6d790ff](https://github.com/y3owk1n/mimi/commit/6d790ff2882e2ab619bd05683a0dfceaf613868a))


### Performance Improvements

* reduce polling overhead, fix event loss and timer races, consolidate duplicated utilities ([#28](https://github.com/y3owk1n/mimi/issues/28)) ([e96befe](https://github.com/y3owk1n/mimi/commit/e96befe8801d2ab36c07ff1d7511b2e2d1f53a07))

## [0.4.0](https://github.com/y3owk1n/mimi/compare/v0.3.1...v0.4.0) (2026-06-05)


### Features

* **event:** add `on_window_resize` observer ([#22](https://github.com/y3owk1n/mimi/issues/22)) ([7c3c7bf](https://github.com/y3owk1n/mimi/commit/7c3c7bfb1d272d4cb44bdab1d9520c51faf936ab))

## [0.3.1](https://github.com/y3owk1n/mimi/compare/v0.3.0...v0.3.1) (2026-06-03)


### Bug Fixes

* cleaner logging ([#20](https://github.com/y3owk1n/mimi/issues/20)) ([4c6c300](https://github.com/y3owk1n/mimi/commit/4c6c300cb94e2d23bd0eafd310235cd4c2776c2a))
* ensure default-config is empty ([50a69e4](https://github.com/y3owk1n/mimi/commit/50a69e4c6a2f0c3b05414355580692788aa2698d))
* **observers:** resolve window focus observer registration regression ([#21](https://github.com/y3owk1n/mimi/issues/21)) ([ff63df8](https://github.com/y3owk1n/mimi/commit/ff63df8be2fcbfa4a8b19f91d80dc4f282538a2a))
* only run the observers if defined ([#18](https://github.com/y3owk1n/mimi/issues/18)) ([13c0f3d](https://github.com/y3owk1n/mimi/commit/13c0f3d012647ed3549285b18e273842ee9fd476))

## [0.3.0](https://github.com/y3owk1n/mimi/compare/v0.2.0...v0.3.0) (2026-06-03)


### Features

* add simple workspace number for systray ([#15](https://github.com/y3owk1n/mimi/issues/15)) ([f3e3e6e](https://github.com/y3owk1n/mimi/commit/f3e3e6ebd0bf826e1b45cae22917a8399bba28da))


### Bug Fixes

* **nix:** set PATH for launchd agent hooks ([#16](https://github.com/y3owk1n/mimi/issues/16)) ([5aeac04](https://github.com/y3owk1n/mimi/commit/5aeac0436ccf281ed99c346eec91a8aca3e62551))
* run permission alerts on main thread + headless Cocoa loop without systray ([#13](https://github.com/y3owk1n/mimi/issues/13)) ([676ae2d](https://github.com/y3owk1n/mimi/commit/676ae2daf2d15a1339c9372badf21be4ff7de8fb))

## [0.2.0](https://github.com/y3owk1n/mimi/compare/v0.1.0...v0.2.0) (2026-06-01)


### Features

* add accesibility prompt flow with tcc reset ([#7](https://github.com/y3owk1n/mimi/issues/7)) ([522df04](https://github.com/y3owk1n/mimi/commit/522df0422a2b2f14489720e779b78e7138ff1c8e))
* add config init prompt on startup ([#8](https://github.com/y3owk1n/mimi/issues/8)) ([ab94093](https://github.com/y3owk1n/mimi/commit/ab940939e54a4407bd078639c483303082698013))
* add systray menubar ([#9](https://github.com/y3owk1n/mimi/issues/9)) ([64653f7](https://github.com/y3owk1n/mimi/commit/64653f79af8ba55e0fd4886dbd01bf5355ed3a52))


### Bug Fixes

* **nix:** wrong launch command ([17001fb](https://github.com/y3owk1n/mimi/commit/17001fb408a8d396306d8135b78b1e7482dce823))
* optimise memory allocation and prevent leak ([#12](https://github.com/y3owk1n/mimi/issues/12)) ([7afd71c](https://github.com/y3owk1n/mimi/commit/7afd71cda72030e150e637908be49f9fb857b742))

## [0.1.0](https://github.com/y3owk1n/mimi/compare/v0.0.0...v0.1.0) (2026-06-01)


### Features

* add more events ([f1511ba](https://github.com/y3owk1n/mimi/commit/f1511ba4350a4d3cd027dc9c8bf08dfff4d93351))
* add nix packages ([c876c70](https://github.com/y3owk1n/mimi/commit/c876c70d5eed1f4309485492f76c2e49f34f17d0))
* **cli:** support `--config` flag for `start` command ([aa16bdd](https://github.com/y3owk1n/mimi/commit/aa16bddb2490cabeb42b8e75d8a31325a7d97216))
* generate manpages ([03baad8](https://github.com/y3owk1n/mimi/commit/03baad8a60523735b65db8099526c88cc0ffcb99))
* initial implementation ([6b0447a](https://github.com/y3owk1n/mimi/commit/6b0447a5f29f136855765fdc5152558411a9d365))
* **logger:** use `zap` and `lumberjact` for logging ([43fe508](https://github.com/y3owk1n/mimi/commit/43fe508566314eb67463053cd9a99d49348deb8b))


### Bug Fixes

* embed default config ([1bf15f5](https://github.com/y3owk1n/mimi/commit/1bf15f52a7415a382a6f7bf1523c4c7d6412db54))
* **event.workspace:** use polling method and expose window info ([e5a6711](https://github.com/y3owk1n/mimi/commit/e5a6711a2ac93da3724476e018f53b79fd90dc3f))
* formatting and linting ([5b8b8de](https://github.com/y3owk1n/mimi/commit/5b8b8de0e2b8b33bba1dc264736cc64aced8d34b))
* **hooks:** ensure replacing event vars ([1659049](https://github.com/y3owk1n/mimi/commit/16590499cec8c615f2625bd91d77efc11699d2fa))
* properly cast `char *` ([493de46](https://github.com/y3owk1n/mimi/commit/493de46576233bf8fc76b2b1e7ddbf17aded13da))
* threading, memory leak, and correctness fixes ([#5](https://github.com/y3owk1n/mimi/issues/5)) ([494bc03](https://github.com/y3owk1n/mimi/commit/494bc03b20e94779ea8804b26d31145043077c90))


### Documentation

* improve installation ([224ffc6](https://github.com/y3owk1n/mimi/commit/224ffc6907da68624fba9e02e4c1148bba490741))
* init simple docs ([da24d2c](https://github.com/y3owk1n/mimi/commit/da24d2cd2f711666f6c66c3702dc97c7ac376ff8))
