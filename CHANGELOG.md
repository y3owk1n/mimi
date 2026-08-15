# Changelog

## [0.10.0](https://github.com/y3owk1n/mimi/compare/v0.9.2...v0.10.0) (2026-08-15)


### Features

* **cli:** cancel the running command on Ctrl-C instead of only killing the process ([#182](https://github.com/y3owk1n/mimi/issues/182)) ([bee3ee7](https://github.com/y3owk1n/mimi/commit/bee3ee7f545d95f381d4897d4897d81832a1e406))
* **config,service:** let settings.service_path set the PATH the installed service runs with ([#167](https://github.com/y3owk1n/mimi/issues/167)) ([d1784ee](https://github.com/y3owk1n/mimi/commit/d1784eef962a2c9ba238fbc2e90582171fdbd3cd))
* **daemon,config:** report the restart-only settings a reload could not apply ([#149](https://github.com/y3owk1n/mimi/issues/149)) ([818d585](https://github.com/y3owk1n/mimi/commit/818d585a5e3d229773d030a4f00cc71e63cffec2))
* **daemon,service,cli:** keep the installed service's captured console logs bounded ([#168](https://github.com/y3owk1n/mimi/issues/168)) ([76117fe](https://github.com/y3owk1n/mimi/commit/76117fe659d16a6bd2f725613736d4b3ec5fd98e))
* **events,daemon:** count event-bus drops and log them on shutdown ([#111](https://github.com/y3owk1n/mimi/issues/111)) ([c5e5e7d](https://github.com/y3owk1n/mimi/commit/c5e5e7df70238d1e46b9b1f48fa948adcc41469a))
* **service,cli:** tell a running service apart from a merely loaded one ([#162](https://github.com/y3owk1n/mimi/issues/162)) ([c5e9c2f](https://github.com/y3owk1n/mimi/commit/c5e9c2fe12d88c4b132df1b197b37a9b075b4d90))
* **systray:** show the last config reload's outcome in the tray menu ([#151](https://github.com/y3owk1n/mimi/issues/151)) ([f48b6fe](https://github.com/y3owk1n/mimi/commit/f48b6fe901dab19f7f43d53a57bdce72dd3f23de))


### Bug Fixes

* **action,cli:** reject --margin and --no-margin given together ([#143](https://github.com/y3owk1n/mimi/issues/143)) ([b5c1aec](https://github.com/y3owk1n/mimi/commit/b5c1aecaf679ed30c9b2321611c7656f38e70d8b))
* **action,cli:** reject a negative --width or --height instead of ignoring it ([#110](https://github.com/y3owk1n/mimi/issues/110)) ([550dda6](https://github.com/y3owk1n/mimi/commit/550dda6f0e72c1fc44d780e2a22c697afc2f172a))
* **action,cli:** trim a resize preset name in the rule, not at the CLI ([#136](https://github.com/y3owk1n/mimi/issues/136)) ([7a97ed7](https://github.com/y3owk1n/mimi/commit/7a97ed71f675f834dc504bd18f11db570339bcd6))
* **action,ipc:** refresh the systray title on the direct-execution path too ([#100](https://github.com/y3owk1n/mimi/issues/100)) ([fb11325](https://github.com/y3owk1n/mimi/commit/fb11325dceed990879b4744e91e9fb564c91db3d))
* **cli:** count hooks of every kind in config validate ([#89](https://github.com/y3owk1n/mimi/issues/89)) ([c099358](https://github.com/y3owk1n/mimi/commit/c099358be0922142e69010dc0f05bc2a544f6f34))
* **cli:** print the usage block only when the command line was wrong ([#181](https://github.com/y3owk1n/mimi/issues/181)) ([e5d6918](https://github.com/y3owk1n/mimi/commit/e5d6918ecb8b1bad8853f8b673b6372213c89137))
* **cli:** resolve the default config path for every command ([#88](https://github.com/y3owk1n/mimi/issues/88)) ([9f09697](https://github.com/y3owk1n/mimi/commit/9f096978b7bbcde61bb31ddb001851332ed1b2f8))
* **config,cli,daemon:** reject unrecognised hook keys and unify hook validation errors ([#123](https://github.com/y3owk1n/mimi/issues/123)) ([544410e](https://github.com/y3owk1n/mimi/commit/544410e4b7cc322d7fb513d6ca8226e7025efa85))
* **config,observe,hooks,systray:** tolerate a nil logger in every constructor that accepts one ([#119](https://github.com/y3owk1n/mimi/issues/119)) ([a889ffb](https://github.com/y3owk1n/mimi/commit/a889ffb38f176e1be66fd57fd973eb4f5c248967))
* **config:** stop the watcher claiming a reload succeeded before it has ([#114](https://github.com/y3owk1n/mimi/issues/114)) ([6be2a62](https://github.com/y3owk1n/mimi/commit/6be2a62fc43636f91152968c98d69f8782c405c8))
* **daemon:** report a failed config reload identically on every trigger ([#104](https://github.com/y3owk1n/mimi/issues/104)) ([8b6679f](https://github.com/y3owk1n/mimi/commit/8b6679f173bc1cc510b4eb5a9952f33e70069803))
* **errors:** correct the IPC error code string and sync the documented list ([#115](https://github.com/y3owk1n/mimi/issues/115)) ([7ae5d00](https://github.com/y3owk1n/mimi/commit/7ae5d00afa6b19ad7edfb3523bcb5616f0763b38))
* **geometry:** give every flush window edge its full tiled margin ([#77](https://github.com/y3owk1n/mimi/issues/77)) ([2ad9ca1](https://github.com/y3owk1n/mimi/commit/2ad9ca16b6963e86d4c9fcb550299daa4bcd6f71))
* **geometry:** honor an explicit --anchor given alongside a resize preset ([#74](https://github.com/y3owk1n/mimi/issues/74)) ([ada0ba2](https://github.com/y3owk1n/mimi/commit/ada0ba2ed7a23db85d2fd023d667956d0141ccf8))
* **geometry:** keep the requested window size when margins would leave nothing ([#76](https://github.com/y3owk1n/mimi/issues/76)) ([b7f680a](https://github.com/y3owk1n/mimi/commit/b7f680a92faada9f4a2ff0e2fe9e6e346768dbbe))
* **hooks,observe:** stop logging hook commands and window titles at debug level ([#116](https://github.com/y3owk1n/mimi/issues/116)) ([62a44e8](https://github.com/y3owk1n/mimi/commit/62a44e8ca440ae47f864214d3f69ade812fe74c2))
* **hooks:** stop logging hook commands and failing hook output at the default log level ([#120](https://github.com/y3owk1n/mimi/issues/120)) ([45fde14](https://github.com/y3owk1n/mimi/commit/45fde14591c9f3b674c0280106fd24ac5fe48ba8))
* **ipc,cli:** detect a daemon on an older wire protocol and say so ([#135](https://github.com/y3owk1n/mimi/issues/135)) ([2dc8eee](https://github.com/y3owk1n/mimi/commit/2dc8eeedf1c63bea992f4c178d4b22323bf2227b))
* **logging:** honor log_format for the console encoder ([#87](https://github.com/y3owk1n/mimi/issues/87)) ([437c1fe](https://github.com/y3owk1n/mimi/commit/437c1fefa096f67733147fd3e28fa7c92536002c))
* **nix,daemon:** keep the module-installed service's captured console logs bounded ([#178](https://github.com/y3owk1n/mimi/issues/178)) ([c11ea3c](https://github.com/y3owk1n/mimi/commit/c11ea3c72dc4d483dba42905c55bc8571dfa289c))
* **paths:** return path unchanged when home directory is unresolvable ([#113](https://github.com/y3owk1n/mimi/issues/113)) ([f21c785](https://github.com/y3owk1n/mimi/commit/f21c785400b8147e84fd10bc418823d115504ad5))
* **service,cli:** fail services uninstall when a loaded service will not unload ([#163](https://github.com/y3owk1n/mimi/issues/163)) ([4c48a48](https://github.com/y3owk1n/mimi/commit/4c48a48adc8b936c3b292d1dff6ac3e3951cd124))
* **service,cli:** make services install replace a stale plist instead of refusing ([#159](https://github.com/y3owk1n/mimi/issues/159)) ([21c55be](https://github.com/y3owk1n/mimi/commit/21c55be9f5f4e9477ef61587ea0bedc032048126))
* **service,cli:** restart the service through launchd instead of a discarded stop ([#174](https://github.com/y3owk1n/mimi/issues/174)) ([e3df34f](https://github.com/y3owk1n/mimi/commit/e3df34f195e012aa3319172f2898e58cd78a67c8))
* **service,cli:** stop reading a launchctl that cannot run as a service that is not loaded ([#171](https://github.com/y3owk1n/mimi/issues/171)) ([e925922](https://github.com/y3owk1n/mimi/commit/e925922b6bf521dcab65ad656b6ba676db40e7d4))
* **service,cli:** wait for the old service to unload before loading the new plist ([#165](https://github.com/y3owk1n/mimi/issues/165)) ([b322a9f](https://github.com/y3owk1n/mimi/commit/b322a9fb049434dcd3476c55c8f8d99eb4a0b2fb))
* **service,cli:** write the installed service's console log beside log_file ([#121](https://github.com/y3owk1n/mimi/issues/121)) ([e9a3d30](https://github.com/y3owk1n/mimi/commit/e9a3d30a2fa0a9364249e9933abf955214a13d02))
* **service:** escape every value substituted into the launchd plist ([#176](https://github.com/y3owk1n/mimi/issues/176)) ([39f8297](https://github.com/y3owk1n/mimi/commit/39f82978d18599aa47f866b1fd1241d07c4fcf98))
* **service:** stop the install refusal naming installs it never checked for ([#180](https://github.com/y3owk1n/mimi/issues/180)) ([693669f](https://github.com/y3owk1n/mimi/commit/693669f3a4d61c01984e9e5b5ffd013ecec4b1f2))
* **service:** stop the installed plist mangling a path named after one of its placeholders ([#179](https://github.com/y3owk1n/mimi/issues/179)) ([00ea4fc](https://github.com/y3owk1n/mimi/commit/00ea4fc4b7dca7144963b2b6fba7347fda5ebf9b))
* **space,native:** restore space switching on macOS 27 ([#150](https://github.com/y3owk1n/mimi/issues/150)) ([8443800](https://github.com/y3owk1n/mimi/commit/8443800e3259bd19fcfdfd99ddd1ce0bb87578cf))
* **systray:** report a requested reload, not a completed one ([#148](https://github.com/y3owk1n/mimi/issues/148)) ([2521318](https://github.com/y3owk1n/mimi/commit/2521318464132097af970a9f58fb64a0776a2448))


### Documentation

* **architecture:** name both conditions that fall back to direct execution ([#141](https://github.com/y3owk1n/mimi/issues/141)) ([34fb1a6](https://github.com/y3owk1n/mimi/commit/34fb1a67f58d05a82836f9dc1313a5c3d803bfcf))
* record reinstall-only as a third reloadability ([#158](https://github.com/y3owk1n/mimi/issues/158)) ([b421fc1](https://github.com/y3owk1n/mimi/commit/b421fc1d09e3f239dce1be9002c72423410a73e8))
* record the domain glossary and the typed daemon wire decision ([995f6b7](https://github.com/y3owk1n/mimi/commit/995f6b78573a5374b7c3caa1b1bc2aaa5a8933a1))

## [0.9.2](https://github.com/y3owk1n/mimi/compare/v0.9.1...v0.9.2) (2026-07-16)


### Bug Fixes

* **action,space:** activate destination display after cross-display window move ([#51](https://github.com/y3owk1n/mimi/issues/51)) ([5026856](https://github.com/y3owk1n/mimi/commit/50268569167ed642fc7be5c3f286c1bc314e8c8d))
* **action,space:** add inter-swipe delays and more fixes ([#54](https://github.com/y3owk1n/mimi/issues/54)) ([4a8a7c9](https://github.com/y3owk1n/mimi/commit/4a8a7c92cd7074303e2a7ba02d227ab0ad5991ee))

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
