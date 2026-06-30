# Changelog

## [0.9.1](https://github.com/y3owk1n/mimi/compare/v0.9.0...v0.9.1) (2026-06-30)


### Bug Fixes

* fix lint, fmt and test after bump deps ([#45](https://github.com/y3owk1n/mimi/issues/45)) ([8add429](https://github.com/y3owk1n/mimi/commit/8add429e154d1c016ebadb2c751b99b401e955e4))

## [0.9.0](https://github.com/y3owk1n/mimi/compare/v0.8.0...v0.9.0) (2026-06-14)


### Features

* **daemon:** make resize debounce duration configurable ([1409c14](https://github.com/y3owk1n/mimi/commit/1409c14f0bb1d3aefe7b135d00c418564b6de6b8))
* **status:** surface native event drop counter ([12dfca3](https://github.com/y3owk1n/mimi/commit/12dfca30557d9b58d06f615ed7874f106f091ac6))


### Bug Fixes

* **axobserver:** filter window create/destroy events to real top-level windows ([16679af](https://github.com/y3owk1n/mimi/commit/16679afd705abd0294a6bd24a7bb2665c5036bb1))
* **axobserver:** log AXObserverAddNotification errors via NSLog ([0612285](https://github.com/y3owk1n/mimi/commit/0612285c00d4b01afd53bda5ea5be430db50debe))
* **ipc:** bound actionCh send with timeout to prevent DoS ([7357630](https://github.com/y3owk1n/mimi/commit/7357630bf98e2c8bb47c34b5b34b314b150ebad7))
* **ipc:** close actionCh on shutdown to release action worker ([4daad38](https://github.com/y3owk1n/mimi/commit/4daad38e8a85a7ecfaa9e6e551fef0493f8ea898))
* **ipc:** recover from action worker panic so clients don't hang ([887c3d1](https://github.com/y3owk1n/mimi/commit/887c3d18c7367b70ca5c81af01074f9ab0b16855))
* **ipc:** use errors.Is(err, net.ErrClosed) instead of string match ([fe7231f](https://github.com/y3owk1n/mimi/commit/fe7231ff366d283a21eacdbe031444063964af31))
* **native:** log AXUIElementSetAttributeValue failures in MimiSetWindowFrame ([778795e](https://github.com/y3owk1n/mimi/commit/778795e3996b4bdda09149bc77ed0784c7d9b11c))
* **native:** release focusedWindow unconditionally in window collector ([04b6187](https://github.com/y3owk1n/mimi/commit/04b6187f4309615ca8e0e1abd5901a5c1c656bd5))
* **native:** use [NSRunningApplication activateWithOptions:0] in window focus ([4b33193](https://github.com/y3owk1n/mimi/commit/4b33193308b56c651afd2176f95fd4cf3fa5e23c))
* **nix:** ensure default config path is correct ([d8d7af2](https://github.com/y3owk1n/mimi/commit/d8d7af2d6a518a56a1ba9ec3a4a89f8c88bd5440))
* **workspace:** only fire workspace_changed from NSWorkspaceActiveSpaceDidChangeNotification ([ea8791d](https://github.com/y3owk1n/mimi/commit/ea8791d33f03588bbfd5cd06a289e494a5e887ac))


### Performance Improvements

* **bus:** skip sends for events with no matching hooks ([9b3cdaf](https://github.com/y3owk1n/mimi/commit/9b3cdafef49c10c6e3fffe6135278ba879c36bad))
* **config:** use resettable timer in config watcher ([c8e1b19](https://github.com/y3owk1n/mimi/commit/c8e1b196e6a92f085c86d93e2a98664787f420d7))
* **focus:** return focused window index from native enumeration ([00749fd](https://github.com/y3owk1n/mimi/commit/00749fd481b756f044f1298064a422f3b15b15e3))
* **hooks:** avoid per-event env map in variable substitution ([76d6f5d](https://github.com/y3owk1n/mimi/commit/76d6f5d50afbb096e17ca95e96dafbb587993528))
* **hooks:** cap hook output capture at 64 KiB ([e435af5](https://github.com/y3owk1n/mimi/commit/e435af537246c9ff11e2ef5abd105691e52a7521))
* **hooks:** precompile app/bundle glob regexes ([2539b0d](https://github.com/y3owk1n/mimi/commit/2539b0d3c7c265a0b87f4fedcc276234e7cd7651))
* **hooks:** precompute os.Environ() once in executor ([224fa86](https://github.com/y3owk1n/mimi/commit/224fa86b482a89f51eede91582ec3e98cf3a5cfe))


### Documentation

* nicer installation guide ([6e08830](https://github.com/y3owk1n/mimi/commit/6e08830b265627999387976a652047c1eb5b5fa7))

## [0.8.0](https://github.com/y3owk1n/mimi/compare/v0.7.0...v0.8.0) (2026-06-11)


### Features

* **action:** add directional to `focus_window` ([#42](https://github.com/y3owk1n/mimi/issues/42)) ([c46f0d6](https://github.com/y3owk1n/mimi/commit/c46f0d69682314bf787c50afd44688a5b0dd8a7c))

## [0.7.0](https://github.com/y3owk1n/mimi/compare/v0.6.0...v0.7.0) (2026-06-09)


### Features

* **cli:** add `resize_window` action ([#36](https://github.com/y3owk1n/mimi/issues/36)) ([4358ffd](https://github.com/y3owk1n/mimi/commit/4358ffd9812dee698ec3d3698a07cd51748b1cc9))


### Bug Fixes

* ensure correct swipe count for space switch in multi display setup ([#40](https://github.com/y3owk1n/mimi/issues/40)) ([dba9acb](https://github.com/y3owk1n/mimi/commit/dba9acb12e14cc019a624c1a407d372d1a8172a8))
* proper `resize_window` coordinate conversion ([#39](https://github.com/y3owk1n/mimi/issues/39)) ([7bc84eb](https://github.com/y3owk1n/mimi/commit/7bc84eb0b307b5ddbe4cf24492f3c5691d9a6594))
* update menubar space number when switching between displays ([#41](https://github.com/y3owk1n/mimi/issues/41)) ([ec88968](https://github.com/y3owk1n/mimi/commit/ec88968c91d2ad5f326148153bb71ff4d1cd1ed9))

## [0.6.0](https://github.com/y3owk1n/mimi/compare/v0.5.0...v0.6.0) (2026-06-08)


### Features

* add `next/prev` cycle with wrapping to space related commands ([#34](https://github.com/y3owk1n/mimi/issues/34)) ([8ae6620](https://github.com/y3owk1n/mimi/commit/8ae6620866dc73ea0795379dde839da5b33d1c37))
* **hook:** add `application` lifecycle hooks ([#32](https://github.com/y3owk1n/mimi/issues/32)) ([d401641](https://github.com/y3owk1n/mimi/commit/d401641b11faedf290b07bcdbe86a4acc1267bef))


### Bug Fixes

* show restart prompt after granting accessibility permission ([#35](https://github.com/y3owk1n/mimi/issues/35)) ([ccc1314](https://github.com/y3owk1n/mimi/commit/ccc131496057a3f84052e7e1853970a9466ca7f2))

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
