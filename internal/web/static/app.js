(function () {
    'use strict';

    /* ======================================================================
       Constants
       ====================================================================== */

    var WS_RECONNECT_DELAY = 3000;
    var RESTART_RELOAD_DELAY = 3000;
    var TOAST_DEFAULT_DURATION = 4000;
    var MSE_RECONNECT_DELAY = 2000;

    var STORAGE_KEY_TOKEN = 'mibee-eye:token';
    var STORAGE_KEY_USER  = 'mibee-eye:user';
    var STORAGE_KEY_THEME = 'mibee-eye:theme';
    var STORAGE_KEY_LANG  = 'mibee-eye:lang';
    var STORAGE_KEY_SIDEBAR = 'mibee-eye:sidebar';

    /* Must match camera.ParamRanges keys (PascalCase ONVIF names) */
    var IMAGING_SLIDERS = [
        { name: 'Brightness',  key: 'imaging.brightness',  fallbackMin: -1, fallbackMax: 1,   fallbackStep: 0.1 },
        { name: 'Contrast',    key: 'imaging.contrast',    fallbackMin:  0, fallbackMax: 32,  fallbackStep: 0.5 },
        { name: 'Saturation',  key: 'imaging.saturation',  fallbackMin:  0, fallbackMax: 32,  fallbackStep: 0.5 },
        { name: 'Sharpness',   key: 'imaging.sharpness',   fallbackMin:  0, fallbackMax: 16,  fallbackStep: 0.5 }
    ];

    var AWB_MODES = ['auto', 'incandescent', 'tungsten', 'fluorescent', 'daylight', 'cloudy', 'custom'];
    var EXPOSURE_MODES = ['normal', 'sport', 'short', 'long', 'custom'];

    /* ======================================================================
       i18n — Inline translation bundles
       ====================================================================== */

    var I18N = {
        en: {
            'a11y.skipToContent': 'Skip to content',
            'brand.tagline': 'Camera Admin',

            'nav.server': 'Server',
            'nav.camera': 'Camera',

            'status.connected': 'Connected',
            'status.disconnected': 'Disconnected',
            'status.reconnecting': 'Reconnecting…',

            'theme.toggle': 'Toggle theme',
            'lang.switch': 'Switch language',
            'lang.current': 'EN',

            'login.subtitle': 'Sign in to manage your camera',
            'login.username': 'Username',
            'login.password': 'Password',
            'login.submit': 'Sign in',
            'login.submitting': 'Signing in…',
            'login.hint': 'Default credentials are the ONVIF username/password.',
            'login.invalidCredentials': 'Invalid username or password',
            'login.fieldsRequired': 'Please enter both username and password',
            'login.networkError': 'Connection failed. Check the device address.',
            'login.sessionExpired': 'Your session has expired. Please sign in again.',

            'actions.signOut': 'Sign out',

            'server.title': 'Server Configuration',
            'server.subtitle': 'Live view of running service configuration.',
            'server.editOnvif': 'Edit ONVIF Credentials',

            'camera.preview': 'Live Preview',
            'camera.previewSub': 'Live video over WebSocket.',
            'camera.connecting': 'Connecting…',
            'camera.live': 'LIVE',
            'camera.imaging': 'Imaging Controls',
            'camera.imagingSub': 'Tune sensor parameters in real time.',

            'imaging.brightness': 'Brightness',
            'imaging.contrast': 'Contrast',
            'imaging.saturation': 'Saturation',
            'imaging.sharpness': 'Sharpness',
            'imaging.awb': 'White Balance',
            'imaging.exposure': 'Exposure Mode',
            'imaging.hflip': 'Horizontal Flip',
            'imaging.vflip': 'Vertical Flip',


            'actions.save': 'Save',
            'actions.cancel': 'Cancel',
            'actions.saving': 'Saving…',
            'actions.saveRestart': 'Save & Restart',
            'actions.close': 'Close',

            'modal.editOnvif': 'Edit ONVIF Credentials',
            'modal.username': 'Username',
            'modal.password': 'Password',
            'modal.userRequired': 'Username is required',
            'modal.passwordRequired': 'Password is required',

            'restart.message': 'Restarting service… Page will reload automatically.',

            'toast.configLoad': { title: 'Load failed', msg: 'Failed to load config: {err}' },
            'toast.imagingLoad': { title: 'Load failed', msg: 'Failed to load imaging controls: {err}' },
            'toast.paramError': { title: 'Parameter error', msg: '{name}: {err}' },
            'toast.presetAdd': { title: 'Preset error', msg: 'Add preset error: {err}' },
            'toast.presetGoto': { title: 'Goto error', msg: 'Goto preset error: {err}' },
            'toast.presetDelete': { title: 'Delete error', msg: 'Delete preset error: {err}' },
            'toast.saved': { title: 'Saved', msg: 'Settings updated, restarting…', kind: 'success' },

            'errors.loadFailed': 'Failed to load',
            'errors.retry': 'Retry',
            'errors.networkError': 'Network error',

            'camera.resetDefaults': 'Reset Defaults',
            'camera.resetConfirm': 'Reset all imaging parameters to defaults?',

            'modal.confirmOnvifSave': 'This will restart the server. Continue?',
            'modal.confirmPresetDelete': 'Are you sure you want to delete this preset?',

        },

        zh: {
            'a11y.skipToContent': '跳到主要内容',
            'brand.tagline': '相机管理',

            'nav.server': '服务器',
            'nav.camera': '相机',

            'status.connected': '已连接',
            'status.disconnected': '未连接',
            'status.reconnecting': '重新连接中…',

            'theme.toggle': '切换主题',
            'lang.switch': '切换语言',
            'lang.current': '中',

            'login.subtitle': '登录以管理你的相机',
            'login.username': '用户名',
            'login.password': '密码',
            'login.submit': '登录',
            'login.submitting': '登录中…',
            'login.hint': '默认凭据为 ONVIF 用户名和密码。',
            'login.invalidCredentials': '用户名或密码错误',
            'login.fieldsRequired': '请输入用户名和密码',
            'login.networkError': '连接失败，请检查设备地址。',
            'login.sessionExpired': '会话已过期，请重新登录。',

            'actions.signOut': '退出登录',

            'server.title': '服务器配置',
            'server.subtitle': '实时查看当前服务配置。',
            'server.editOnvif': '编辑 ONVIF 凭据',

            'camera.preview': '实时预览',
            'camera.previewSub': '通过 WebSocket 实现实时视频。',
            'camera.connecting': '连接中…',
            'camera.live': '直播',
            'camera.imaging': '图像控制',
            'camera.imagingSub': '实时调节传感器参数。',

            'imaging.brightness': '亮度',
            'imaging.contrast': '对比度',
            'imaging.saturation': '饱和度',
            'imaging.sharpness': '锐度',
            'imaging.awb': '白平衡',
            'imaging.exposure': '曝光模式',
            'imaging.hflip': '水平翻转',
            'imaging.vflip': '垂直翻转',


            'actions.save': '保存',
            'actions.cancel': '取消',
            'actions.saving': '保存中…',
            'actions.saveRestart': '保存并重启',
            'actions.close': '关闭',

            'modal.editOnvif': '编辑 ONVIF 凭据',
            'modal.username': '用户名',
            'modal.password': '密码',
            'modal.userRequired': '用户名不能为空',
            'modal.passwordRequired': '密码不能为空',

            'restart.message': '服务正在重启… 页面将自动刷新。',

            'toast.configLoad': { title: '加载失败', msg: '无法加载配置：{err}' },
            'toast.imagingLoad': { title: '加载失败', msg: '无法加载图像控制：{err}' },
            'toast.paramError': { title: '参数错误', msg: '{name}：{err}' },
            'toast.presetAdd': { title: '预置位错误', msg: '添加预置位失败：{err}' },
            'toast.presetGoto': { title: '调用错误', msg: '调用预置位失败：{err}' },
            'toast.presetDelete': { title: '删除错误', msg: '删除预置位失败：{err}' },
            'toast.saved': { title: '已保存', msg: '设置已更新，正在重启…', kind: 'success' },

            'errors.loadFailed': '加载失败',
            'errors.retry': '重试',
            'errors.networkError': '网络错误',

            'camera.resetDefaults': '重置默认值',
            'camera.resetConfirm': '确定要将所有图像参数重置为默认值吗？',

            'modal.confirmOnvifSave': '此操作将重启服务器，确定继续吗？',
            'modal.confirmPresetDelete': '确定要删除此预置位吗？',

        }
    };

    /* ======================================================================
       State
       ====================================================================== */

    var state = {
        lang: 'en',
        theme: 'dark',
        token: null,
        username: null,
        currentTab: 'server',
        ws: null,
        wsReconnectTimer: null,
        mseActive: false,
        msePlaying: false,
    };

    /* ======================================================================
       DOM helpers
       ====================================================================== */

    function $(sel) { return document.querySelector(sel); }
    function $$(sel) { return document.querySelectorAll(sel); }

    function el(tag, attrs, children) {
        var e = document.createElement(tag);
        if (attrs) {
            Object.keys(attrs).forEach(function (k) {
                if (k === 'className') e.className = attrs[k];
                else if (k === 'textContent') e.textContent = attrs[k];
                else if (k === 'innerHTML') e.innerHTML = attrs[k];
                else if (k === 'dataset' && typeof attrs[k] === 'object') {
                    Object.keys(attrs[k]).forEach(function (dk) { e.dataset[dk] = attrs[k][dk]; });
                }
                else if (k.indexOf('on') === 0) e.addEventListener(k.slice(2).toLowerCase(), attrs[k]);
                else e.setAttribute(k, attrs[k]);
            });
        }
        if (children) {
            (Array.isArray(children) ? children : [children]).forEach(function (c) {
                if (c == null) return;
                if (typeof c === 'string' || typeof c === 'number') e.appendChild(document.createTextNode(String(c)));
                else e.appendChild(c);
            });
        }
        return e;
    }

    function renderSkeleton(container, lines) {
        if (!container) return;
        var sk = el('div', { className: 'skeleton' });
        var count = lines || 3;
        for (var i = 0; i < count; i++) {
            sk.appendChild(el('div', { className: 'skeleton-line' }));
        }
        container.innerHTML = '';
        container.appendChild(sk);
    }

    function renderErrorState(container, msg, retryFn) {
        if (!container) return;
        container.innerHTML = '';
        var state = el('div', { className: 'error-state' });
        state.appendChild(el('div', { className: 'error-icon', innerHTML: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M15 9l-6 6M9 9l6 6"/></svg>' }));
        state.appendChild(el('div', { className: 'error-msg', textContent: msg || t('errors.loadFailed') }));
        if (retryFn) {
            state.appendChild(el('button', { className: 'btn btn-sm btn-ghost', textContent: t('errors.retry'), onClick: retryFn }));
        }
        container.appendChild(state);
    }

    /* ======================================================================
       i18n
       ====================================================================== */

    function detectLang() {
        var stored = localStorage.getItem(STORAGE_KEY_LANG);
        if (stored && I18N[stored]) return stored;
        var nav = (navigator.language || 'en').toLowerCase();
        return nav.indexOf('zh') === 0 ? 'zh' : 'en';
    }

    function detectTheme() {
        var stored = localStorage.getItem(STORAGE_KEY_THEME);
        if (stored === 'light' || stored === 'dark') return stored;
        if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) return 'light';
        return 'dark';
    }

    function t(key, vars) {
        var bundle = I18N[state.lang] || I18N.en;
        var s = bundle[key];
        if (s === undefined) s = I18N.en[key] !== undefined ? I18N.en[key] : key;
        if (typeof s !== 'string') return s;
        if (vars) {
            Object.keys(vars).forEach(function (k) {
                s = s.split('{' + k + '}').join(vars[k]);
            });
        }
        return s;
    }

    function applyI18n() {
        document.documentElement.setAttribute('lang', state.lang);
        document.documentElement.setAttribute('data-lang', state.lang);

        $$('[data-i18n]').forEach(function (e) {
            var key = e.getAttribute('data-i18n');
            var v = t(key);
            if (typeof v === 'string') e.textContent = v;
        });
        $$('[data-i18n-aria]').forEach(function (e) {
            var key = e.getAttribute('data-i18n-aria');
            var v = t(key);
            if (typeof v === 'string') e.setAttribute('aria-label', v);
        });
        $$('[data-i18n-title]').forEach(function (e) {
            var key = e.getAttribute('data-i18n-title');
            var v = t(key);
            if (typeof v === 'string') e.setAttribute('title', v);
        });
        $$('[data-i18n-placeholder]').forEach(function (e) {
            var key = e.getAttribute('data-i18n-placeholder');
            var v = t(key);
            if (typeof v === 'string') e.setAttribute('placeholder', v);
        });

        var langCurrent = $('.lang-current');
        if (langCurrent) langCurrent.textContent = t('lang.current');

        var pageTitle = $('#page-title');
        if (pageTitle) {
            var key = state.currentTab === 'camera' ? 'nav.camera' : 'nav.server';
            pageTitle.textContent = t(key);
        }
    }

    function setLang(lang) {
        if (!I18N[lang]) return;
        state.lang = lang;
        localStorage.setItem(STORAGE_KEY_LANG, lang);
        applyI18n();
    }

    function cycleLang() {
        setLang(state.lang === 'en' ? 'zh' : 'en');
    }

    /* ======================================================================
       Theme
       ====================================================================== */

    function setTheme(theme) {
        state.theme = theme;
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem(STORAGE_KEY_THEME, theme);
    }

    function toggleTheme() {
        setTheme(state.theme === 'dark' ? 'light' : 'dark');
    }

    /* Listen for system theme changes */
    if (window.matchMedia) {
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function (e) {
            if (localStorage.getItem(STORAGE_KEY_THEME)) return;
            setTheme(e.matches ? 'dark' : 'light');
        });
    }

    /* ======================================================================
       Auth — token, login, logout
       ====================================================================== */

    function loadToken() {
        try {
            state.token = localStorage.getItem(STORAGE_KEY_TOKEN);
            state.username = localStorage.getItem(STORAGE_KEY_USER);
        } catch (e) { /* localStorage unavailable */ }
    }

    function saveToken(token, username) {
        state.token = token;
        state.username = username;
        try {
            localStorage.setItem(STORAGE_KEY_TOKEN, token);
            localStorage.setItem(STORAGE_KEY_USER, username);
        } catch (e) { /* localStorage unavailable */ }
    }

    function clearToken() {
        state.token = null;
        state.username = null;
        try {
            localStorage.removeItem(STORAGE_KEY_TOKEN);
            localStorage.removeItem(STORAGE_KEY_USER);
        } catch (e) { /* localStorage unavailable */ }
    }

    function setAuth(authed) {
        document.body.setAttribute('data-auth', authed ? 'logged-in' : 'logged-out');
        var app = $('.app[data-auth-content]');
        if (app) {
            if (authed) app.removeAttribute('hidden');
            else app.setAttribute('hidden', '');
        }
        var userName = $('#user-name');
        if (userName && state.username) userName.textContent = state.username;
    }

    function doLogin(username, password) {
        return fetch('/api/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username: username, password: password })
        }).then(function (r) {
            if (!r.ok) {
                if (r.status === 401) {
                    var err = new Error(t('login.invalidCredentials'));
                    err.code = 'INVALID_CREDENTIALS';
                    throw err;
                }
                return r.text().then(function (txt) {
                    var msg = txt || r.statusText;
                    try {
                        var parsed = JSON.parse(txt);
                        if (parsed && parsed.error) msg = parsed.error;
                    } catch (e) { /* keep txt */ }
                    var err2 = new Error(msg);
                    err2.code = 'HTTP_' + r.status;
                    throw err2;
                });
            }
            return r.json();
        }).then(function (data) {
            if (!data || !data.token) {
                throw new Error('No token in response');
            }
            saveToken(data.token, data.username || username);
            return data;
        });
    }

    function doLogout() {
        if (state.token) {
            /* Best-effort server logout, ignore failures */
            fetch('/api/logout', {
                method: 'POST',
                headers: { 'Authorization': 'Bearer ' + state.token }
            }).catch(function () { /* silent */ });
        }
        clearToken();
        destroyMsePlayer();
        closeWS();
        setAuth(false);
        /* Reset form */
        var form = $('#login-form');
        if (form) form.reset();
        var err = $('#login-error');
        if (err) { err.textContent = ''; err.setAttribute('hidden', ''); }
        setTimeout(function () {
            var u = $('#login-user');
            if (u) u.focus();
        }, 100);
    }

    function initLogin() {
        var form = $('#login-form');
        var errBox = $('#login-error');
        var submitBtn = $('#login-submit');
        var userInput = $('#login-user');
        var passInput = $('#login-pass');
        var passToggle = $('#login-pass-toggle');

        if (!form) return;

        function showError(msg) {
            errBox.textContent = msg;
            errBox.removeAttribute('hidden');
        }
        function clearError() {
            errBox.textContent = '';
            errBox.setAttribute('hidden', '');
        }

        passToggle.addEventListener('click', function () {
            var visible = passInput.type === 'text';
            passInput.type = visible ? 'password' : 'text';
            passToggle.classList.toggle('is-visible', !visible);
            passToggle.setAttribute('aria-label', visible ? 'Show password' : 'Hide password');
        });

        form.addEventListener('submit', function (e) {
            e.preventDefault();
            clearError();

            var username = userInput.value.trim();
            var password = passInput.value;

            if (!username || !password) {
                showError(t('login.fieldsRequired'));
                return;
            }

            submitBtn.classList.add('is-loading');
            submitBtn.disabled = true;

            doLogin(username, password)
                .then(function () {
                    submitBtn.classList.remove('is-loading');
                    submitBtn.disabled = false;
			setAuth(true);
			/* Initialize the app after login */
			bootApp();
			/* Restart MSE player if camera tab is active (re-login after session expiry) */
			if (state.currentTab === 'camera') {
				destroyMsePlayer();
				startMsePlayer();
			}
		})
		.catch(function (err) {
                    submitBtn.classList.remove('is-loading');
                    submitBtn.disabled = false;
                    showError(err.message || t('login.networkError'));
                });
        });

        setTimeout(function () { userInput.focus(); }, 50);
    }

    function initLogout() {
        var btn = $('#btn-logout');
        if (btn) btn.addEventListener('click', doLogout);
    }

    /* ======================================================================
       API helpers
       ====================================================================== */

    function api(method, url, body) {
        var opts = {
            method: method,
            headers: {}
        };
        if (state.token) {
            opts.headers['Authorization'] = 'Bearer ' + state.token;
        }
        if (body !== undefined) {
            opts.headers['Content-Type'] = 'application/json';
            opts.body = JSON.stringify(body);
        }
	return fetch(url, opts).then(function (r) {
		if (r.status === 401) {
			/* Token expired or invalid — return to login without destroying MSE player.
			   The player will be restarted when the user re-logs in. */
			closeWS();
			clearToken();
			setAuth(false);
			var err = new Error(t('login.sessionExpired'));
			err.status = 401;
			throw err;
		}
            if (r.status === 401) {
                /* Token expired or invalid — return to login */
                destroyMsePlayer();
                closeWS();
                clearToken();
                setAuth(false);
                var err = new Error(t('login.sessionExpired'));
                err.status = 401;
                throw err;
            }
            if (!r.ok) {
                return r.text().then(function (txt) {
                    var msg = txt || r.statusText;
                    try {
                        var parsed = JSON.parse(txt);
                        if (parsed && parsed.error) msg = parsed.error;
                    } catch (e) { /* keep txt */ }
                    var err2 = new Error(msg);
                    err2.status = r.status;
                    throw err2;
                });
            }
            var ct = r.headers.get('content-type') || '';
            if (ct.indexOf('json') !== -1) return r.json();
            return null;
        });
    }

    /* ======================================================================
       Sidebar
       ====================================================================== */

    function initSidebar() {
        var collapsed = localStorage.getItem(STORAGE_KEY_SIDEBAR) === 'collapsed';
        if (collapsed) document.body.classList.add('sidebar-collapsed');

        $$('.nav-item').forEach(function (btn) {
            btn.addEventListener('click', function () {
                var tab = btn.dataset.tab;
                switchTab(tab);
                if (window.innerWidth <= 900) {
                    document.body.classList.remove('sidebar-open');
                }
            });
        });

        var toggleBtn = $('#btn-sidebar-toggle');
        if (toggleBtn) {
            toggleBtn.addEventListener('click', function () {
                if (window.innerWidth <= 900) {
                    document.body.classList.toggle('sidebar-open');
                } else {
                    document.body.classList.toggle('sidebar-collapsed');
                    localStorage.setItem(STORAGE_KEY_SIDEBAR,
                        document.body.classList.contains('sidebar-collapsed') ? 'collapsed' : 'expanded');
                }
            });
        }

        /* Close mobile sidebar when clicking outside (on the ::after backdrop) */
        document.addEventListener('click', function (e) {
            if (window.innerWidth > 900) return;
            if (!document.body.classList.contains('sidebar-open')) return;
            var sidebar = $('.sidebar');
            if (sidebar && sidebar.contains(e.target)) return;
            var tb = $('#btn-sidebar-toggle');
            if (tb && tb.contains(e.target)) return;
            document.body.classList.remove('sidebar-open');
            if (tb) tb.setAttribute('aria-expanded', 'false');
        });
    }

    function switchTab(tab) {
        state.currentTab = tab;
        $$('.nav-item').forEach(function (b) {
            b.classList.toggle('active', b.dataset.tab === tab);
        });
        $$('.tab-panel').forEach(function (p) {
            var isActive = p.id === 'tab-' + tab;
            p.classList.toggle('active', isActive);
            if (isActive) p.removeAttribute('hidden');
            else p.setAttribute('hidden', '');
        });
        var pageTitle = $('#page-title');
        if (pageTitle) {
            pageTitle.textContent = t(tab === 'camera' ? 'nav.camera' : 'nav.server');
        }
        if (tab === 'camera') {
            if (!state.imagingRendered) loadImaging();
            startMsePlayer();
        }
        if (tab !== 'camera') {
            destroyMsePlayer();
        }
    }

    /* ======================================================================
       Toast
       ====================================================================== */

    function showToast(keyOrText, vars, opts) {
        var title, msg, kind = 'info', duration = TOAST_DEFAULT_DURATION;
        if (typeof keyOrText === 'string' && I18N.en[keyOrText] !== undefined) {
            var def = t(keyOrText, vars);
            if (typeof def === 'object' && def !== null) {
                title = def.title;
                msg = def.msg;
                kind = def.kind || 'info';
            } else {
                title = def;
            }
            if (opts && opts.kind) kind = opts.kind;
            if (opts && opts.duration) duration = opts.duration;
        } else if (vars && typeof vars === 'object' && !opts) {
            title = keyOrText;
            msg = null;
            Object.keys(vars).forEach(function (k) {
                title = title.split('{' + k + '}').join(vars[k]);
            });
        } else {
            title = keyOrText;
        }

        var stack = $('#toast-stack');
        if (!stack) return;

        var node = el('div', { className: 'toast toast-' + kind, role: 'status' });
        var iconSvg = {
            info: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/>',
            success: '<path d="M20 6L9 17l-5-5"/>',
            warning: '<path d="M12 9v4M12 17h.01M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>',
            error: '<circle cx="12" cy="12" r="10"/><path d="M15 9l-6 6M9 9l6 6"/>'
        }[kind] || '<circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/>';

        // Build SVG icon element
        var svg = el('svg', { className: 'toast-icon', viewBox: '0 0 24 24' });
        svg.innerHTML = iconSvg; // iconSvg is from hardcoded static lookup — safe

        // Build toast body
        var body = el('div', { className: 'toast-body' });
        if (title) body.appendChild(el('div', { className: 'toast-title' }, title));
        if (msg) body.appendChild(el('div', { className: 'toast-msg' }, msg));

        // Build close button
        var closeBtn = el('button', { className: 'toast-close', type: 'button', 'aria-label': 'Close' }, '\u00d7');

        node.appendChild(svg);
        node.appendChild(body);
        node.appendChild(closeBtn);
        closeBtn.addEventListener('click', function () {
            clearTimeout(dismissTimer);
            node.classList.add('is-leaving');
            setTimeout(function () {
                if (node.parentNode) node.parentNode.removeChild(node);
            }, 250);
        });

        var dismissTimer = setTimeout(function () {
            node.classList.add('is-leaving');
            setTimeout(function () {
                if (node.parentNode) node.parentNode.removeChild(node);
            }, 250);
        }, duration);

        stack.appendChild(node);
    }

    /* ======================================================================
       Server Tab — Config Display
       ====================================================================== */

    function loadConfig() {
        var grid = $('#server-config');
        renderSkeleton(grid, 4);
        api('GET', '/api/config').then(renderConfig).catch(function (err) {
            if (err.status !== 401) {
                renderErrorState(grid, t('toast.configLoad', { err: err.message }).msg, loadConfig);
            }
        });
    }

    function renderConfig(data) {
        var grid = $('#server-config');
        if (!grid) return;
        grid.innerHTML = '';

        var sections = ['camera', 'rtsp', 'onvif', 'rtmp', 'device', 'logging', 'web'];
        sections.forEach(function (sec) {
            var obj = data[sec];
            if (!obj || typeof obj !== 'object') return;

            var card = el('div', { className: 'config-card' });
            card.appendChild(el('h3', { className: 'config-card-title', textContent: sec.toUpperCase() }));

            Object.keys(obj).forEach(function (key) {
                var val = obj[key];
                if (val === null || val === undefined) val = '';
                if (typeof val === 'object') val = JSON.stringify(val);

                var row = el('div', { className: 'config-row' });
                row.appendChild(el('span', { className: 'config-key', textContent: key }));
                var valSpan = el('span', { className: 'config-val mono' });

                if (key === 'password') {
                    valSpan.textContent = val ? '\u2022'.repeat(8) : '';
                } else {
                    valSpan.textContent = String(val);
                }
                row.appendChild(valSpan);
                card.appendChild(row);
            });

            grid.appendChild(card);
        });
    }

    /* ======================================================================
       ONVIF Modal
       ====================================================================== */

    function initOnvifModal() {
        var overlay = $('#modal-overlay');
        var btnEdit = $('#btn-edit-onvif');
        var btnSave = $('#btn-save-onvif');
        var btnCancel = $('#btn-cancel-onvif');
        var btnClose = $('#btn-modal-close');
        var errBox = $('#modal-error');
        var inputUser = $('#input-onvif-user');
        var inputPass = $('#input-onvif-pass');
        var previousFocus = null;

        function openModal() {
            previousFocus = document.activeElement;
            errBox.setAttribute('hidden', '');
            errBox.textContent = '';
            inputUser.value = '';
            inputPass.value = '';
            overlay.removeAttribute('hidden');
            setTimeout(function () { inputUser.focus(); }, 50);
        }

        function closeModal() {
            overlay.setAttribute('hidden', '');
            if (previousFocus && previousFocus.focus) previousFocus.focus();
        }

        /* Focus trap: cycle Tab within modal */
        overlay.addEventListener('keydown', function (e) {
            if (e.key !== 'Tab') return;
            var focusable = overlay.querySelectorAll(
                'input:not([disabled]):not([type="hidden"]), button:not([disabled]), [tabindex]:not([tabindex="-1"])'
            );
            if (focusable.length === 0) return;
            var first = focusable[0];
            var last = focusable[focusable.length - 1];
            if (e.shiftKey) {
                if (document.activeElement === first) {
                    e.preventDefault();
                    last.focus();
                }
            } else {
                if (document.activeElement === last) {
                    e.preventDefault();
                    first.focus();
                }
            }
        });

        if (btnEdit) btnEdit.addEventListener('click', openModal);
        if (btnCancel) btnCancel.addEventListener('click', closeModal);
        if (btnClose) btnClose.addEventListener('click', closeModal);

        overlay.addEventListener('click', function (e) {
            if (e.target === overlay) closeModal();
        });

        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape' && !overlay.hasAttribute('hidden')) closeModal();
        });

        btnSave.addEventListener('click', function () {
            var username = inputUser.value.trim();
            var password = inputPass.value;

            if (!username) {
                errBox.textContent = t('modal.userRequired');
                errBox.removeAttribute('hidden');
                inputUser.focus();
                return;
            }

            if (!password) {
                errBox.textContent = t('modal.passwordRequired');
                errBox.removeAttribute('hidden');
                inputPass.focus();
                return;
            }

            var confirmMsg = t('modal.confirmOnvifSave');
            showConfirmModal(confirmMsg).then(function () {
                btnSave.classList.add('is-loading');
                btnSave.disabled = true;

                api('POST', '/api/config/onvif', { username: username, password: password })
                    .then(function () {
                        closeModal();
                        showToast('toast.saved', null, { kind: 'success' });
                        showRestartBanner();
                    })
                    .catch(function (err) {
                        if (err.status !== 401) {
                            errBox.textContent = err.message;
                            errBox.removeAttribute('hidden');
                        }
                    })
                    .finally(function () {
                        btnSave.classList.remove('is-loading');
                        btnSave.disabled = false;
                    });
            }).catch(function () { /* cancelled — do nothing */ });
        });
    }

    function hideLoading() {
        var el = $('#loading-overlay');
        if (el) el.setAttribute('hidden', '');
    }

    function showConfirmModal(message) {
        var overlay = $('#confirm-overlay');
        var msgEl = $('#confirm-message');
        var btnYes = $('#btn-confirm-yes');
        var btnNo = $('#btn-confirm-no');
        var btnClose = $('#btn-confirm-close');
        var previousFocus = document.activeElement;

        return new Promise(function (resolve, reject) {
            function cleanup() {
                overlay.setAttribute('hidden', '');
                btnYes.removeEventListener('click', onConfirm);
                btnNo.removeEventListener('click', onCancel);
                btnClose.removeEventListener('click', onCancel);
                overlay.removeEventListener('click', onOverlayClick);
                document.removeEventListener('keydown', onKeydown);
                if (previousFocus && previousFocus.focus) previousFocus.focus();
            }
            function onConfirm() { cleanup(); resolve(); }
            function onCancel() { cleanup(); reject(); }
            function onOverlayClick(e) { if (e.target === overlay) onCancel(); }
            function onKeydown(e) { if (e.key === 'Escape') onCancel(); }

            msgEl.textContent = message;
            btnYes.addEventListener('click', onConfirm);
            btnNo.addEventListener('click', onCancel);
            btnClose.addEventListener('click', onCancel);
            overlay.addEventListener('click', onOverlayClick);
            document.addEventListener('keydown', onKeydown);

            overlay.removeAttribute('hidden');
            setTimeout(function () { btnYes.focus(); }, 50);
        });
    }

    function showRestartBanner() {
        var banner = $('#restart-banner');
        banner.removeAttribute('hidden');
        setTimeout(function () { window.location.reload(); }, RESTART_RELOAD_DELAY);
    }

    /* ======================================================================
       Camera Tab — WebSocket + MSE Live Video Player
       ====================================================================== */

    /* --- fMP4 Muxer --- */

    function str(s) {
        var b = new Uint8Array(s.length);
        for (var i = 0; i < s.length; i++) b[i] = s.charCodeAt(i);
        return b;
    }

    function u32(v) {
        var b = new Uint8Array(4);
        new DataView(b.buffer).setUint32(0, v >>> 0);
        return b;
    }

    function u16(v) {
        var b = new Uint8Array(2);
        new DataView(b.buffer).setUint16(0, v);
        return b;
    }

    function u8(v) {
        return new Uint8Array([v]);
    }

    function concat(arrays) {
        var total = 0;
        for (var i = 0; i < arrays.length; i++) total += arrays[i].byteLength;
        var r = new Uint8Array(total);
        var off = 0;
        for (var i = 0; i < arrays.length; i++) {
            r.set(arrays[i], off);
            off += arrays[i].byteLength;
        }
        return r;
    }

    function box(type, contents) {
        var payload = Array.isArray(contents) ? concat(contents) : contents;
        var len = 8 + payload.byteLength;
        var b = new Uint8Array(len);
        var view = new DataView(b.buffer);
        view.setUint32(0, len);
        b.set(str(type), 4);
        b.set(payload instanceof Uint8Array ? payload : new Uint8Array(payload), 8);
        return b;
    }

    // Parse SPS NALU to extract actual video dimensions
    function parseSpsDimensions(sps) {
        try {
            var data = sps;
            var profileIdc = data[1];
            var i = 4; // skip NALU header(1) + profile(1) + constraints(1) + level(1)

            // Bit reader
            var bitPos = 0;
            function readBit() {
                var byteIdx = (i * 8 + bitPos) >> 3;
                var bitIdx = 7 - ((i * 8 + bitPos) & 7);
                // Re-calculate from absolute position
                byteIdx = i + Math.floor(bitPos / 8);
                bitIdx = 7 - (bitPos % 8);
                if (byteIdx >= data.length) return 0;
                bitPos++;
                return (data[byteIdx] >> bitIdx) & 1;
            }
            function readUE() {
                var leadingZeros = 0;
                while (readBit() === 0 && leadingZeros < 32) leadingZeros++;
                if (leadingZeros === 0) return 0;
                var val = 1;
                for (var b = 0; b < leadingZeros; b++) val = (val << 1) | readBit();
                return val - 1;
            }
            function readU(n) {
                var val = 0;
                for (var b = 0; b < n; b++) val = (val << 1) | readBit();
                return val;
            }

            // Reset bit reader to after level_idc (byte offset 4)
            i = 4; bitPos = 0;

            var spsId = readUE();

            // High profile (100) has extra fields
            var chromaFormatIdc = 1;
            if (profileIdc === 100 || profileIdc === 110 || profileIdc === 122 ||
                profileIdc === 244 || profileIdc === 44 || profileIdc === 83 ||
                profileIdc === 86 || profileIdc === 118 || profileIdc === 128 ||
                profileIdc === 138 || profileIdc === 139 || profileIdc === 134 ||
                profileIdc === 135) {
                chromaFormatIdc = readUE();
                if (chromaFormatIdc === 3) readU(1); // separate_colour_plane_flag
                readUE(); // bit_depth_luma_minus8
                readUE(); // bit_depth_chroma_minus8
                readU(1); // qpprime_y_zero_transform_bypass_flag
                var spsScaling = readU(1); // seq_scaling_matrix_present_flag
                if (spsScaling) {
                    var nLists = chromaFormatIdc === 3 ? 12 : 8;
                    for (var li = 0; li < nLists; li++) {
                        if (readU(1)) { // scaling_list_present_flag
                            // skip scaling list (hard to parse, just bail)
                            return null;
                        }
                    }
                }
            }

            readUE(); // log2_max_frame_num_minus4
            var picOrderCntType = readUE();
            if (picOrderCntType === 0) {
                readUE(); // log2_max_pic_order_cnt_lsb_minus4
            } else if (picOrderCntType === 1) {
                readU(1); // delta_pic_order_always_zero_flag
                readUE(); // offset_for_non_ref_pic
                readUE(); // offset_for_top_to_bottom_field
                var nRef = readUE(); // num_ref_frames_in_pic_order_cnt_cycle
                for (var ri = 0; ri < nRef; ri++) readUE();
            }

            readUE(); // max_num_ref_frames
            readU(1); // gaps_in_frame_num_value_allowed_flag
            var picWidthInMbsMinus1 = readUE();
            var picHeightInMapUnitsMinus1 = readUE();
            var frameMbsOnlyFlag = readU(1);

            var width = (picWidthInMbsMinus1 + 1) * 16;
            var height = (2 - frameMbsOnlyFlag) * (picHeightInMapUnitsMinus1 + 1) * 16;

            console.log('[MSE] SPS parsed: ' + width + 'x' + height + ' (mbs=' + (picWidthInMbsMinus1+1) + 'x' + (picHeightInMapUnitsMinus1+1) + ' frameMbsOnly=' + frameMbsOnlyFlag + ')');
            return { width: width, height: height };
        } catch (e) {
            console.warn('[MSE] SPS parse failed:', e.message);
            return null;
        }
    }

    function buildAvcc(sps, pps) {
        return box('avcC', [
            u8(1),                          // configurationVersion
            u8(sps[1]),                     // AVCProfileIndication
            u8(sps[2]),                     // profile_compatibility
            u8(sps[3]),                     // AVCLevelIndication
            u8(0xFC | 3),                   // lengthSizeMinusOne (4 bytes)
            u8(0xE0 | 1),                   // numOfSequenceParameterSets
            u16(sps.byteLength),            // SPS length
            sps,                            // SPS data
            u8(1),                          // numOfPictureParameterSets
            u16(pps.byteLength),            // PPS length
            pps                             // PPS data
        ]);
    }

    function buildInitSegment(sps, pps, trackId) {
        var ts = 90000;
        var dims = parseSpsDimensions(sps) || { width: 1280, height: 720 };
        var w = dims.width, h = dims.height;

        // mvhd
        var mvhd = box('mvhd', [
            u32(0), u32(0), u32(0),          // version=0, ctime, mtime
            u32(ts),                         // timescale
            u32(0),                          // duration (live)
            u32(0x00010000),                 // rate 1.0
            u16(0x0100), u16(0),             // volume, reserved
            u32(0), u32(0),                  // reserved
            // matrix (identity)
            u32(0x00010000), u32(0), u32(0),
            u32(0), u32(0x00010000), u32(0),
            u32(0), u32(0), u32(0x40000000),
            u32(0), u32(0), u32(0), u32(0), u32(0), u32(0), // pre-defined
            u32(2)                           // next track ID
        ]);

        // tkhd
        var tkhd = box('tkhd', [
            u32(0x0003),                     // enabled | inMovie
            u32(0), u32(0),                  // ctime, mtime
            u32(trackId),                    // track ID
            u32(0),                          // reserved
            u32(0),                          // duration
            u32(0), u32(0),                  // reserved
            u16(0), u16(0),                  // layer, alternate_group
            u16(0), u16(0),                  // volume, reserved
            // matrix
            u32(0x00010000), u32(0), u32(0),
            u32(0), u32(0x00010000), u32(0),
            u32(0), u32(0), u32(0x40000000),
            u32(w << 16),                    // width
            u32(h << 16)                     // height
        ]);

        // mdhd
        var mdhd = box('mdhd', [
            u32(0), u32(0), u32(0),
            u32(ts),                         // timescale
            u32(0),                          // duration
            u16(0x55C4), u16(0)              // language 'und', pre-defined
        ]);

        // hdlr
        var hdlr = box('hdlr', [
            u32(0), u32(0), str('vide'),
            u32(0), u32(0), u32(0),            // reserved
            str('VideoHandler\x00')                // name (null-terminated)
        ]);

        // vmhd
        var vmhd = box('vmhd', [
            u32(0x0001),                     // version=0, flags=1
            u16(0), u16(0), u16(0), u16(0)   // graphics mode, opcolor
        ]);

        // dref
        var dref = box('dref', [u32(0), u32(1), box('url ', u32(0x0001))]);
        var dinf = box('dinf', dref);

        // avcC + avc1 + stsd
        var avcC = buildAvcc(sps, pps);
        var avc1 = box('avc1', [
            u32(0), u16(0), u16(1),          // reserved, data_ref_index
            u16(0), u16(0),                   // pre-defined
            str('\x00\x00\x00\x00'),          // vendor
            u32(0), u32(0),                   // temporal, spatial quality
            u16(w), u16(h),                   // width, height
            u32(0x00480000), u32(0x00480000), // resolution 72dpi
            u32(0), u16(1),                   // data_size, frame_count
            str('\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00' +
                 '\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00'), // compressorname
            u16(0x0018), u16(0xFFFF),         // depth, pre-defined
            avcC
        ]);
        var stsd = box('stsd', [u32(0), u32(1), avc1]);

        // empty tables (fragmented)
        var stts = box('stts', [u32(0), u32(0)]);
        var stsc = box('stsc', [u32(0), u32(0)]);
        var stsz = box('stsz', [u32(0), u32(0), u32(0)]);
        var stco = box('stco', [u32(0), u32(0)]);

        var stbl = box('stbl', [stsd, stts, stsc, stsz, stco]);
        var minf = box('minf', [vmhd, dinf, stbl]);
        var mdia = box('mdia', [mdhd, hdlr, minf]);
        var trak = box('trak', [tkhd, mdia]);

        // mvex + trex
        var trex = box('trex', [u32(0), u32(trackId), u32(1), u32(0), u32(0), u32(0)]);
        var mvex = box('mvex', trex);
        var moov = box('moov', [mvhd, trak, mvex]);

        // ftyp
        var ftyp = box('ftyp', [str('isom'), u32(0x200), str('isom'), str('iso2'), str('avc1'), str('mp41')]);

        return concat([ftyp, moov]);
    }

    function buildFragment(frameNalus, isIDR, seqNum, decodeTime, trackId, duration) {
        // Convert NALUs to AVCC format (4-byte length prefix)
        var avccParts = [];
        for (var i = 0; i < frameNalus.length; i++) {
            var n = frameNalus[i];
            avccParts.push(u32(n.byteLength));
            avccParts.push(n);
        }
        var avccData = concat(avccParts);
        var sampleSize = avccData.byteLength;
        if (seqNum === 1) {
            console.log('[MSE] buildFragment: nalus=' + frameNalus.length + ' nalu0_len=' + frameNalus[0].byteLength + ' nalu0_first=0x' + frameNalus[0][0].toString(16));
            console.log('[MSE] buildFragment: avccData len=' + avccData.byteLength + ' first8:', Array.from(avccData.slice(0, 8)).map(function(b) { return b.toString(16).padStart(2, '0'); }).join(' '));
        }
        var sampleFlags = isIDR ? 0x02000000 : 0x01010000;

        // tfdt
        var tfdt = box('tfdt', [
            u8(1), u8(0), u8(0), u8(0),      // version=1, flags=0
            u32(Math.floor(decodeTime / 4294967296)),
            u32(decodeTime >>> 0)
        ]);

        // tfhd
        var tfhd = box('tfhd', [u32(0x020000), u32(trackId)]); // default-base-is-moof

        // mfhd
        var mfhd = box('mfhd', [u32(0), u32(seqNum)]);

        // trun
        // trun (data-offset patched after moof size is known)
        var trun = box('trun', [
            u32(0x000701),                   // version=0, flags: data-offset+duration+size+flags
            u32(1),                          // sample count
            u32(0),                          // placeholder data-offset
            u32(duration),                   // sample duration
            u32(sampleSize),                 // sample size
            u32(sampleFlags)                 // sample flags
        ]);

        var traf = box('traf', [tfhd, tfdt, trun]);
        var moof = box('moof', [mfhd, traf]);

        // Patch data-offset: offset from moof start to mdat payload
        // moof size is deterministic, so rebuild with correct value
        var dataOffset = moof.byteLength + 8; // 8 = mdat box header (size + type)
        trun = box('trun', [
            u32(0x000701),
            u32(1),
            u32(dataOffset),                 // correct data-offset
            u32(duration),
            u32(sampleSize),
            u32(sampleFlags)
        ]);
        traf = box('traf', [tfhd, tfdt, trun]);
        moof = box('moof', [mfhd, traf]);

        var mdat = box('mdat', avccData);

        return concat([moof, mdat]);
    }

    /* --- Annex-B Parser --- */

    function parseAnnexB(data) {
        var nalus = [];
        var i = 0;
        var start = -1;
        while (i < data.length) {
            if (i + 3 < data.length && data[i] === 0 && data[i+1] === 0 && data[i+2] === 0 && data[i+3] === 1) {
                if (start >= 0) nalus.push(data.slice(start, i));
                start = i + 4;
                i += 4;
            } else if (i + 2 < data.length && data[i] === 0 && data[i+1] === 0 && data[i+2] === 1) {
                if (start >= 0) nalus.push(data.slice(start, i));
                start = i + 3;
                i += 3;
            } else {
                i++;
            }
        }
        if (start >= 0) nalus.push(data.slice(start));
        return nalus;
    }

    function naluType(nalu) { return nalu[0] & 0x1F; }

    /* --- MSE Player --- */

    var msePlayer = null;

    function startMsePlayer() {
        if (!state.token) return;
        if (state.mseActive) return;

        var video = document.getElementById('live-video');
        var placeholder = document.getElementById('video-placeholder');
        var badge = document.getElementById('live-badge');
        if (!video || !placeholder) return;

        if (!window.MediaSource) {
            setMseBadge(badge, 'MSE unsupported');
            return;
        }

        state.mseActive = true;
        state.msePlaying = false;

        var mediaSource = null;
        var sourceBuffer = null;
        var ws = null;
        var reconnectTimer = null;
        var sps = null;
        var pps = null;
        var initSent = false;
        var sequenceNumber = 1;
        var baseDecodeTime = 0;
        var frameCount = 0;
        var trackId = 1;
        var frameDuration = 6000; // 90000/15fps
        var pendingQueue = [];
        var appending = false;
        var destroyed = false;
        var reconnectAttempts = 0;
        var lastFrameTime = 0;

        function cleanupState() {
            if (sourceBuffer && mediaSource && mediaSource.sourceBuffers.length > 0) {
                try { mediaSource.endOfStream(); } catch (e) { /* ignore */ }
            }
            if (mediaSource && mediaSource.readyState === 'open') {
                try { mediaSource.endOfStream(); } catch (e) { /* ignore */ }
            }
            if (ws) {
                try { ws.close(); } catch (e) { /* ignore */ }
                ws = null;
            }
            if (reconnectTimer) {
                clearTimeout(reconnectTimer);
                reconnectTimer = null;
            }
            if (video) {
                if (video.src) {
                    try { URL.revokeObjectURL(video.src); } catch (e) { /* ignore */ }
                    video.src = '';
                }
            }
            pendingQueue = [];
            appending = false;
            sps = null;
            pps = null;
            initSent = false;
            mediaSource = null;
            sourceBuffer = null;
        }

        function setBadgeText(text, isLive) {
            if (!badge) return;
            badge.textContent = text;
            badge.className = 'live-badge' + (isLive ? ' live' : '');
        }

        function hidePlaceholder() {
            if (placeholder) placeholder.style.display = 'none';
        }

        function showPlaceholder() {
            if (placeholder) placeholder.style.display = 'flex';
        }

        function appendToSourceBuffer(data) {
            if (destroyed) return;
            if (!sourceBuffer) return;

            function doAppend() {
                if (destroyed) return;
                try {
                    sourceBuffer.appendBuffer(data);
                    appending = true;
                } catch (e) {
                    // Buffer full or other error, drop frame
                    appending = false;
                    pendingQueue = [];
                }
            }

            if (appending) {
                pendingQueue.push(data);
                return;
            }
            doAppend();
        }

        // SourceBuffer created dynamically in handleData when SPS arrives


        function handleData(data) {
            if (!(data instanceof ArrayBuffer)) return;
            reconnectAttempts = 0;

            var bytes = new Uint8Array(data);
            var nalus = parseAnnexB(bytes);

            var foundSps = null, foundPps = null;
            var frameNalus = [];
            var isIDR = false;

            for (var i = 0; i < nalus.length; i++) {
                var type = naluType(nalus[i]);
                if (type === 7) foundSps = nalus[i];
                else if (type === 8) foundPps = nalus[i];
                else if (type === 5) { isIDR = true; frameNalus.push(nalus[i]); }
                else if (type === 1) frameNalus.push(nalus[i]);
            }

            if (foundSps) sps = foundSps;
            if (foundPps) pps = foundPps;

            // First SPS+PPS: derive codec, create SourceBuffer, send init segment
            if (sps && pps && !initSent) {
                var profileHex = sps[1].toString(16).padStart(2, '0');
                var constraintsHex = sps[2].toString(16).padStart(2, '0');
                var levelHex = sps[3].toString(16).padStart(2, '0');
                var codec = 'avc1.' + profileHex + constraintsHex + levelHex;
                console.log('[MSE] SPS profile=' + sps[1] + ' constraints=' + sps[2] + ' level=' + sps[3] + ' codec=' + codec);
                console.log('[MSE] SPS first 8 bytes:', Array.from(sps.slice(0, 8)).map(function(b) { return b.toString(16).padStart(2, '0'); }).join(' '));

                try {
                    sourceBuffer = mediaSource.addSourceBuffer('video/mp4; codecs="' + codec + '"');
                    sourceBuffer.mode = 'sequence';
                    console.log('[MSE] SourceBuffer created with codec:', codec);
                } catch (e) {
                    console.warn('[MSE] Codec', codec, 'failed:', e, '— trying generic');
                    try {
                        sourceBuffer = mediaSource.addSourceBuffer('video/mp4');
                    } catch (e2) {
                        console.error('[MSE] Fallback also failed:', e2);
                        return;
                    }
                }

                var _playStarted = false;
                sourceBuffer.addEventListener('updateend', function () {
                    appending = false;
                    if (!_playStarted && !destroyed && video.buffered.length > 0) {
                        _playStarted = true;
                        console.log('[MSE] First buffer range ready, calling video.play()');
                        var p = video.play();
                        if (p && p.catch) p.catch(function (e) { console.warn('[MSE] play() rejected:', e.name, e.message); });
                    }
                    if (pendingQueue.length > 0 && !destroyed) {
                        var next = pendingQueue.shift();
                        try {
                            sourceBuffer.appendBuffer(next);
                            appending = true;
                        } catch (e) {
                            console.error('[MSE] Queue appendBuffer failed:', e);
                            pendingQueue = [];
                            appending = false;
                        }
                    }
                });
                sourceBuffer.addEventListener('error', function (e) {
                    console.error('[MSE] SourceBuffer error event:', e.type, 'sb.updating=' + (sourceBuffer ? sourceBuffer.updating : 'null'), 'ms.readyState=' + (mediaSource ? mediaSource.readyState : 'null'));
                });

                var init = buildInitSegment(sps, pps, trackId);
                console.log('[MSE] Appending init segment, size=' + init.byteLength);
                console.log('[MSE] Init hex (full ' + init.byteLength + ' bytes):', Array.from(init).map(function(b) { return b.toString(16).padStart(2, '0'); }).join(' '));
                appendToSourceBuffer(init);
                initSent = true;
            }

            if (!initSent) {
                if (frameCount === 0) console.log('[MSE] Waiting for SPS+PPS... got NALU types:', nalus.map(function(n){return naluType(n);}).join(','));
                return;
            }

            if (frameNalus.length === 0) return;

            // Create and send fragment
            var frag = buildFragment(frameNalus, isIDR, sequenceNumber++, baseDecodeTime, trackId, frameDuration);
            if (frameCount === 0) {
                console.log('[MSE] Frag hex (first 120):', Array.from(frag.slice(0, 120)).map(function(b) { return b.toString(16).padStart(2, '0'); }).join(' '));
            }
            baseDecodeTime += frameDuration;
            frameCount++;
            appendToSourceBuffer(frag);

            if (frameCount <= 3) {
                console.log('[MSE] Frame #' + frameCount + ' isIDR=' + isIDR + ' nalus=' + frameNalus.length + ' fragSize=' + frag.byteLength + ' dts=' + (baseDecodeTime - frameDuration));
            }

            // Update UI
            if (frameCount <= 2) {
                hidePlaceholder();
                setBadgeText(t('camera.live'), true);
                state.msePlaying = true;
            }
            lastFrameTime = Date.now();
        }

        function connectWs() {
            if (destroyed) return;
            setBadgeText(t('camera.connecting'), false);

            var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
            var url = proto + '//' + location.host + '/api/stream/ws?token=' + encodeURIComponent(state.token);

            try {
                ws = new WebSocket(url);
            } catch (e) {
                scheduleReconnect();
                return;
            }

            ws.binaryType = 'arraybuffer';

            ws.onopen = function () {
                setBadgeText(t('camera.connecting'), false);
            };

            ws.onmessage = function (evt) {
                handleData(evt.data);
            };

            ws.onclose = function () {
                setBadgeText(t('status.disconnected'), false);
                if (!destroyed) scheduleReconnect();
            };

            ws.onerror = function () { /* close fires after this */ };
        }

        function scheduleReconnect() {
            if (destroyed) return;
            reconnectAttempts++;
            var delay = Math.min(MSE_RECONNECT_DELAY * Math.min(reconnectAttempts, 5), 10000);
            setBadgeText(t('status.reconnecting'), false);
            if (reconnectTimer) clearTimeout(reconnectTimer);
            reconnectTimer = setTimeout(function () {
                if (!destroyed) connectWs();
            }, delay);
        }

        function destroy() {
            if (destroyed) return;
            destroyed = true;
            state.mseActive = false;
            state.msePlaying = false;
            cleanupState();
            showPlaceholder();
            setBadgeText('\u2014', false);
            video.removeAttribute('src');
        }

        msePlayer = {
            destroy: destroy
        };

        // Create MediaSource
        try {
            mediaSource = new MediaSource();
            video.src = URL.createObjectURL(mediaSource);
        } catch (e) {
            setBadgeText('MSE init error', false);
            state.mseActive = false;
            return;
        }

        mediaSource.addEventListener('sourceopen', function () {
            if (destroyed) return;
            console.log('[MSE] sourceopen — connecting WebSocket');
            connectWs();
        });

        video.addEventListener('error', function (e) {
            var err = e.target.error;
            console.error('[MSE] Video element error: code=' + err.code + ' message="' + (err.message || '') + '"');
        });
    }

    function destroyMsePlayer() {
        if (msePlayer) {
            msePlayer.destroy();
            msePlayer = null;
        }
    }

    /* ======================================================================
       Camera Tab — Imaging Controls
       ====================================================================== */

    function loadImaging() {
        var container = $('#imaging-controls');
        renderSkeleton(container, 6);
        Promise.all([
            api('GET', '/api/camera/params'),
            api('GET', '/api/camera/options')
        ]).then(function (results) {
            renderImaging(results[0] || {}, results[1] || {});
        }).catch(function (err) {
            if (err.status !== 401) {
                renderErrorState(container, t('toast.imagingLoad', { err: err.message }).msg, loadImaging);
            }
        });
    }

    function reloadImaging() {
        if (state.lastImagingParams && state.lastImagingOptions) {
            renderImaging(state.lastImagingParams, state.lastImagingOptions);
        }
    }

    function renderImaging(params, options) {
        state.lastImagingParams = params;
        state.lastImagingOptions = options;
        state.imagingRendered = true;

        var container = $('#imaging-controls');
        if (!container) return;
        container.innerHTML = '';

        IMAGING_SLIDERS.forEach(function (cfg) {
            var current = params[cfg.name] !== undefined ? Number(params[cfg.name]) : 0;
            var range = options[cfg.name] || {};
            var min = range.min !== undefined ? range.min : cfg.fallbackMin;
            var max = range.max !== undefined ? range.max : cfg.fallbackMax;
            var step = range.step !== undefined ? range.step : cfg.fallbackStep;
            var label = t(cfg.key);
            var sliderId = 'imaging-' + cfg.name.toLowerCase();

            var wrap = el('div', { className: 'param-control' });

            var header = el('div', { className: 'param-header' });
            header.appendChild(el('label', { className: 'param-label', textContent: label, 'for': sliderId }));
            var valSpan = el('span', { className: 'param-value mono', textContent: formatNumber(current, step) });
            header.appendChild(valSpan);
            wrap.appendChild(header);

            var slider = el('input', {
                className: 'param-slider',
                type: 'range',
                id: sliderId,
                min: String(min),
                max: String(max),
                step: String(step),
                value: String(current),
                'data-param': cfg.name
            });
            updateSliderFill(slider);

            slider.addEventListener('input', function () {
                valSpan.textContent = formatNumber(Number(slider.value), step);
                updateSliderFill(slider);
            });

            slider.addEventListener('change', function () {
                var v = Number(slider.value);
                valSpan.textContent = formatNumber(v, step);
                postParam(cfg.name, v);
                flashValue(wrap);
            });

            wrap.appendChild(slider);

            var labels = el('div', { className: 'param-range-labels' });
            labels.appendChild(el('span', { textContent: min }));
            labels.appendChild(el('span', { textContent: max }));
            wrap.appendChild(labels);

            container.appendChild(wrap);
        });

        var awbVal = params.AWBMode || 'auto';
        var awbEnums = (options.AWBMode && options.AWBMode.enums) || AWB_MODES;
        container.appendChild(buildSelect(t('imaging.awb'), 'AWBMode', awbVal, awbEnums));

        var expVal = params.ExposureMode || 'normal';
        var expEnums = (options.ExposureMode && options.ExposureMode.enums) || EXPOSURE_MODES;
        container.appendChild(buildSelect(t('imaging.exposure'), 'ExposureMode', expVal, expEnums));

        var bools = [
            { name: 'HFlip', key: 'imaging.hflip' },
            { name: 'VFlip', key: 'imaging.vflip' }
        ];

        bools.forEach(function (b) {
            var on = !!params[b.name];
            var row = el('div', { className: 'param-control param-bool' });
            row.appendChild(el('span', { className: 'param-label', textContent: t(b.key) }));

            var toggle = el('label', { className: 'toggle' });
            var input = el('input', { type: 'checkbox', 'data-param': b.name });
            if (on) input.checked = true;

            input.addEventListener('change', function () {
                postParam(b.name, input.checked);
            });

            toggle.appendChild(input);
            toggle.appendChild(el('span', { className: 'toggle-slider' }));
            row.appendChild(toggle);
            container.appendChild(row);
        });
    }

    function buildSelect(label, name, current, enums) {
        var wrap = el('div', { className: 'param-control' });
        var selId = 'imaging-' + name.toLowerCase();
        wrap.appendChild(el('label', { className: 'param-label', textContent: label, 'for': selId }));

        var sel = el('select', { className: 'param-select', id: selId, 'data-param': name });
        enums.forEach(function (opt) {
            var o = el('option', { value: opt, textContent: opt });
            if (opt === current) o.selected = true;
            sel.appendChild(o);
        });

        sel.addEventListener('change', function () {
            postParam(name, sel.value);
        });

        wrap.appendChild(sel);
        return wrap;
    }

    function postParam(name, value) {
        clearTimeout(state._postParamTimers && state._postParamTimers[name]);
        if (!state._postParamTimers) state._postParamTimers = {};
        state._postParamTimers[name] = setTimeout(function () {
            api('POST', '/api/camera/param', { name: name, value: value }).catch(function (err) {
                if (err.status !== 401) {
                    showToast('toast.paramError', { name: name, err: err.message }, { kind: 'error' });
                }
            });
        }, 150);
    }

    function resetImagingDefaults() {
        showConfirmModal(t('camera.resetConfirm')).then(function () {
            var defaults = {
                'Brightness': 0,
                'Contrast': 1,
                'Saturation': 1,
                'Sharpness': 1,
                'AWBMode': 'auto',
                'ExposureMode': 'normal',
                'HFlip': 0,
                'VFlip': 0
            };
            var names = Object.keys(defaults);
            var i = 0;
            function next() {
                if (i >= names.length) {
                    loadImaging();
                    return;
                }
                var name = names[i];
                var value = defaults[name];
                i++;
                api('POST', '/api/camera/param', { name: name, value: value })
                    .then(next)
                    .catch(function () { next(); });
            }
            next();
        }).catch(function () { /* cancelled */ });
    }

    function flashValue(wrap) {
        wrap.classList.add('flash');
        setTimeout(function () { wrap.classList.remove('flash'); }, 400);
    }

    function updateSliderFill(slider) {
        var min = Number(slider.min) || 0;
        var max = Number(slider.max) || 1;
        var val = Number(slider.value);
        var pct = ((val - min) / (max - min)) * 100;
        slider.style.setProperty('--val', pct + '%');
    }

    function formatNumber(v, step) {
        if (step >= 1) return String(Math.round(v));
        if (step >= 0.1) return v.toFixed(1);
        return v.toFixed(2);
    }

    /* ======================================================================
       WebSocket (Control Channel)
       ====================================================================== */

    function connectWS() {
        if (!state.token) {
            setWSStatus(false);
            return;
        }
        var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        var url = proto + '//' + location.host + '/ws?token=' + encodeURIComponent(state.token);

        try {
            state.ws = new WebSocket(url);
        } catch (e) {
            scheduleReconnect();
            return;
        }

        state.ws.onopen = function () { setWSStatus(true); };
        state.ws.onclose = function () { setWSStatus(false); scheduleReconnect(); };
        state.ws.onerror = function () { /* close fires after this */ };
        state.ws.onmessage = function (evt) { handleWSMessage(evt.data); };
    }

    function closeWS() {
        if (state.ws) {
            try { state.ws.close(); } catch (e) { /* ignore */ }
            state.ws = null;
        }
        clearTimeout(state.wsReconnectTimer);
    }

    function scheduleReconnect() {
        clearTimeout(state.wsReconnectTimer);
        setWSStatus(false, true);
        state.wsReconnectTimer = setTimeout(connectWS, WS_RECONNECT_DELAY);
    }

    function setWSStatus(connected, reconnecting) {
        var badge = $('#ws-status');
        if (!badge) return;
        var text = badge.querySelector('.status-text');
        if (connected) {
            badge.className = 'status-pill connected';
            if (text) text.textContent = t('status.connected');
        } else if (reconnecting) {
            badge.className = 'status-pill reconnecting';
            if (text) text.textContent = t('status.reconnecting');
        } else {
            badge.className = 'status-pill disconnected';
            if (text) text.textContent = t('status.disconnected');
        }
    }

    function handleWSMessage(raw) {
        var msg;
        try { msg = JSON.parse(raw); }
        catch (e) { return; }

        switch (msg.type) {
            case 'ping':
                break;
            case 'param-changed':
                applyParamUpdate(msg.name, msg.value);
                break;
        }
    }


    function applyParamUpdate(name, value) {
        if (!name) return;

        var slider = $('.param-slider[data-param="' + name + '"]');
        if (slider) {
            slider.value = value;
            var valSpan = slider.parentElement.querySelector('.param-value');
            if (valSpan) {
                var step = Number(slider.step) || 0.1;
                valSpan.textContent = formatNumber(Number(value), step);
            }
            updateSliderFill(slider);
            flashValue(slider.parentElement);
            return;
        }

        var sel = $('.param-select[data-param="' + name + '"]');
        if (sel) {
            sel.value = value;
            return;
        }

        var cb = $('input[type="checkbox"][data-param="' + name + '"]');
        if (cb) {
            cb.checked = !!value;
        }
    }

    /* ======================================================================
       Top bar
       ====================================================================== */

    function initTopbar() {
        var themeBtn = $('#btn-theme');
        if (themeBtn) {
            themeBtn.addEventListener('click', toggleTheme);
            themeBtn.setAttribute('title', t('theme.toggle'));
            themeBtn.setAttribute('aria-label', t('theme.toggle'));
        }
        var langBtn = $('#btn-lang');
        if (langBtn) {
            langBtn.addEventListener('click', cycleLang);
            langBtn.setAttribute('title', t('lang.switch'));
            langBtn.setAttribute('aria-label', t('lang.switch'));
        }
        var logoutBtn = $('#btn-logout');
        if (logoutBtn) {
            logoutBtn.setAttribute('title', t('actions.signOut'));
            logoutBtn.setAttribute('aria-label', t('actions.signOut'));
        }
    }

    /* ======================================================================
       Boot
       ====================================================================== */

    function bootApp() {
        /* Only run once per session — guards against double-init from auth flow */
        if (state.initialized) return;
        state.initialized = true;

        initTopbar();
        initSidebar();
        initOnvifModal();
        /* Bind reset defaults button */
        var resetBtn = $('#btn-reset-imaging');
        if (resetBtn) resetBtn.addEventListener('click', resetImagingDefaults);
        initLogout();

		fetchVersion();
        loadConfig();

        applyI18n();
        connectWS();

        /* Clean up on page close */
        window.addEventListener('beforeunload', function () {
            destroyMsePlayer();
            closeWS();
        });
    }

    function fetchVersion() {
        var el = document.getElementById('version-display');
        if (!el) return;
        fetch('/api/version').then(function (r) {
            if (!r.ok) throw new Error('version fetch failed');
            return r.json();
        }).then(function (data) {
            el.textContent = data.version || '--';
        }).catch(function () {
            el.textContent = '--';
        });
    }

    function init() {
        state.theme = detectTheme();
        document.documentElement.setAttribute('data-theme', state.theme);
        state.lang = detectLang();
        loadToken();

        /* Wire up login screen (always present, shows initially) */
        initLogin();

        /* Apply i18n to login screen */
        applyI18n();

        /* If we have a stored token, verify it by hitting /api/config */
        if (state.token) {
            fetch('/api/config', {
                headers: { 'Authorization': 'Bearer ' + state.token }
            }).then(function (r) {
                if (r.ok) {
                    setAuth(true);
                    bootApp();
                } else {
                    clearToken();
                    setAuth(false);
                }
                hideLoading();
            }).catch(function () {
                /* Network error — assume still logged in, show app */
                setAuth(true);
                bootApp();
                hideLoading();
            });
        } else {
            setAuth(false);
            hideLoading();
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
