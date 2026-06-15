(function () {
    'use strict';

    /* ======================================================================
       Constants
       ====================================================================== */

    var SNAPSHOT_INTERVAL = 2000;
    var WS_RECONNECT_DELAY = 3000;
    var RESTART_RELOAD_DELAY = 3000;
    var TOAST_DEFAULT_DURATION = 4000;

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
            'camera.previewSub': 'HLS live stream, ~3s latency.',
            'camera.noSnapshot': 'No snapshot available',
            'camera.connecting': 'Connecting…',
            'camera.live': 'LIVE',
            'camera.imaging': 'Imaging Controls',
            'camera.imagingSub': 'Tune sensor parameters in real time.',
            'camera.ptz': 'PTZ Controls',
            'camera.ptzSub': 'Digital pan, tilt and zoom via software crop.',

            'imaging.brightness': 'Brightness',
            'imaging.contrast': 'Contrast',
            'imaging.saturation': 'Saturation',
            'imaging.sharpness': 'Sharpness',
            'imaging.awb': 'White Balance',
            'imaging.exposure': 'Exposure Mode',
            'imaging.hflip': 'Horizontal Flip',
            'imaging.vflip': 'Vertical Flip',

            'ptz.pan': 'Pan',
            'ptz.tilt': 'Tilt',
            'ptz.zoom': 'Zoom',
            'ptz.status': 'Status',
            'ptz.idle': 'IDLE',
            'ptz.moving': 'MOVING',
            'ptz.stop': 'STOP',
            'ptz.panLeft': 'Pan left',
            'ptz.panRight': 'Pan right',
            'ptz.tiltUp': 'Tilt up',
            'ptz.tiltDown': 'Tilt down',
            'ptz.zoomIn': 'Zoom +',
            'ptz.zoomOut': 'Zoom −',
            'ptz.presets': 'Presets',
            'ptz.presetsSub': 'Save and recall positions for quick access.',
            'ptz.addPreset': 'Add Preset',
            'ptz.presetName': 'Preset',
            'ptz.goto': 'Goto',
            'ptz.delete': 'Delete',
            'ptz.noPresets': 'No presets defined',
            'ptz.presetPos': 'P: {p}  T: {t}  Z: {z}',

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
            'toast.ptzMove': { title: 'PTZ error', msg: 'PTZ move error: {err}' },
            'toast.presetAdd': { title: 'Preset error', msg: 'Add preset error: {err}' },
            'toast.presetGoto': { title: 'Goto error', msg: 'Goto preset error: {err}' },
            'toast.presetDelete': { title: 'Delete error', msg: 'Delete preset error: {err}' },
            'toast.saved': { title: 'Saved', msg: 'Settings updated, restarting…', kind: 'success' }
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
            'camera.previewSub': 'HLS 直播流，约 3 秒延迟。',
            'camera.noSnapshot': '暂无截图',
            'camera.connecting': '连接中…',
            'camera.live': '直播',
            'camera.imaging': '图像控制',
            'camera.imagingSub': '实时调节传感器参数。',
            'camera.ptz': 'PTZ 控制',
            'camera.ptzSub': '通过软件裁剪实现数字云台。',

            'imaging.brightness': '亮度',
            'imaging.contrast': '对比度',
            'imaging.saturation': '饱和度',
            'imaging.sharpness': '锐度',
            'imaging.awb': '白平衡',
            'imaging.exposure': '曝光模式',
            'imaging.hflip': '水平翻转',
            'imaging.vflip': '垂直翻转',

            'ptz.pan': '水平',
            'ptz.tilt': '垂直',
            'ptz.zoom': '缩放',
            'ptz.status': '状态',
            'ptz.idle': '空闲',
            'ptz.moving': '运动中',
            'ptz.stop': '停止',
            'ptz.panLeft': '向左',
            'ptz.panRight': '向右',
            'ptz.tiltUp': '向上',
            'ptz.tiltDown': '向下',
            'ptz.zoomIn': '放大',
            'ptz.zoomOut': '缩小',
            'ptz.presets': '预置位',
            'ptz.presetsSub': '保存和调用位置以便快速访问。',
            'ptz.addPreset': '添加预置位',
            'ptz.presetName': '预置位',
            'ptz.goto': '调用',
            'ptz.delete': '删除',
            'ptz.noPresets': '暂无预置位',
            'ptz.presetPos': '水平: {p}  垂直: {t}  缩放: {z}',

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
            'toast.ptzMove': { title: '云台错误', msg: '云台移动失败：{err}' },
            'toast.presetAdd': { title: '预置位错误', msg: '添加预置位失败：{err}' },
            'toast.presetGoto': { title: '调用错误', msg: '调用预置位失败：{err}' },
            'toast.presetDelete': { title: '删除错误', msg: '删除预置位失败：{err}' },
            'toast.saved': { title: '已保存', msg: '设置已更新，正在重启…', kind: 'success' }
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
        snapshotTimer: null,
        metaTimer: null,
        ws: null,
        wsReconnectTimer: null,
        snapshotLoadTime: 0,
        snapshotFailures: 0,
        hlsInstance: null,
        videoPlaying: false,
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
                /* Token expired or invalid — return to login */
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
            if (!state.ptzRendered) loadPTZ();
            /* Restart snapshot/meta timers when entering camera tab */
            if (!state.snapshotTimer) {
                refreshSnapshot();
                state.snapshotTimer = setInterval(refreshSnapshot, SNAPSHOT_INTERVAL);
                state.metaTimer = setInterval(updateSnapshotMeta, 1000);
            }
        }
        /* Stop HLS playback and snapshot timers when leaving camera tab */
        if (tab !== 'camera') {
            if (state.hlsInstance) {
                try { state.hlsInstance.destroy(); } catch (e) {}
                state.hlsInstance = null;
                state.videoPlaying = false;
            }
            if (state.snapshotTimer) {
                clearInterval(state.snapshotTimer);
                state.snapshotTimer = null;
            }
            if (state.metaTimer) {
                clearInterval(state.metaTimer);
                state.metaTimer = null;
            }
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
        if (!stack) {
            var t0 = $('#toast');
            if (t0) {
                t0.textContent = title + (msg ? ': ' + msg : '');
                t0.classList.remove('hidden');
                setTimeout(function () { t0.classList.add('hidden'); }, duration);
            }
            return;
        }

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
        api('GET', '/api/config').then(renderConfig).catch(function (err) {
            if (err.status !== 401) {
                showToast('toast.configLoad', { err: err.message });
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

            var confirmMsg = state.lang === 'zh' ? '此操作将重启服务器，确定继续吗？' : 'This will restart the server. Continue?';
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
       Camera Tab — Snapshot
       ====================================================================== */

    function initSnapshot() {
        var img = $('#snapshot');
        var placeholder = $('#snapshot-placeholder');
        var meta = $('#snapshot-meta');

        img.addEventListener('load', function () {
            img.hidden = false;
            if (placeholder) placeholder.style.display = 'none';
            state.snapshotLoadTime = Date.now();
            state.snapshotFailures = 0;
            updateSnapshotMeta();
        });

        img.addEventListener('error', function () {
            img.hidden = true;
            if (placeholder) placeholder.style.display = 'flex';
            state.snapshotFailures++;
            updateSnapshotMeta();
        });

        refreshSnapshot();
        if (!state.snapshotTimer) {
            state.snapshotTimer = setInterval(refreshSnapshot, SNAPSHOT_INTERVAL);
            state.metaTimer = setInterval(updateSnapshotMeta, 1000);
        }
    }

    /* ======================================================================
       Camera Tab — HLS Live Video (preferred over JPEG when available)
       ====================================================================== */

    function initLiveVideo() {
        var video = $('#live-video');
        if (!video) return false;
        var img = $('#snapshot');
        var placeholder = $('#snapshot-placeholder');

        function onPlaying() {
            // HLS stream is up — hide JPEG snapshot, show video, mark live.
            if (img) img.hidden = true;
            video.style.display = 'block';
            if (placeholder) placeholder.style.display = 'none';
            state.videoPlaying = true;
            state.snapshotLoadTime = Date.now();
            state.snapshotFailures = 0;
            updateSnapshotMeta();
        }

        function fallbackToSnapshot() {
            // HLS failed — unhide the JPEG snapshot path so the user sees something.
            if (video) video.style.display = 'none';
            if (img) img.hidden = false;
            if (state.hlsInstance) {
                try { state.hlsInstance.destroy(); } catch (e) {}
                state.hlsInstance = null;
            }
            state.videoPlaying = false;
            // If we have never loaded a JPEG, trigger one now.
            if (img && !img.src) refreshSnapshot();
        }

        // hls.js path (Chrome, Firefox, Edge, etc.)
        if (window.Hls && Hls.isSupported()) {
            var hls = new Hls({
                liveSyncDurationCount: 3,
                liveMaxLatencyDurationCount: 6,
                enableWorker: false,
                lowLatencyMode: false,
                xhrSetup: function(xhr) {
                    if (state.token) {
                        xhr.setRequestHeader('Authorization', 'Bearer ' + state.token);
                    }
                },
            });
            state.hlsInstance = hls;
            hls.loadSource('/api/hls/stream.m3u8?token=' + encodeURIComponent(state.token || ''));
            hls.attachMedia(video);

            hls.on(Hls.Events.MANIFEST_PARSED, function () {
                video.play().catch(function () { /* autoplay may be blocked */ });
            });
            video.addEventListener('playing', onPlaying);

            hls.on(Hls.Events.ERROR, function (_event, data) {
                if (!data.fatal) return;
                if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
                    hls.startLoad();
                } else {
                    fallbackToSnapshot();
                }
            });
            
            // Fallback timeout: if HLS hasn't started playing in 8s, use snapshot
            setTimeout(function() {
                if (!state.videoPlaying) {
                    fallbackToSnapshot();
                }
            }, 8000);
            
            // Start snapshot polling immediately as a preview while HLS loads
            refreshSnapshot();
            return true;
        }

        // Native HLS path (Safari)
        if (video.canPlayType('application/vnd.apple.mpegurl')) {
            video.src = '/api/hls/stream.m3u8?token=' + encodeURIComponent(state.token || '');
            video.addEventListener('playing', onPlaying);
            video.addEventListener('error', function () { fallbackToSnapshot(); });
            return true;
        }

        // No HLS support — caller should rely on the JPEG snapshot path.
        return false;
    }

    function refreshSnapshot() {
        if (state.videoPlaying) return;
        var img = $('#snapshot');
        if (!img) return;
        img.src = '/api/snapshot?ts=' + Date.now() + (state.token ? '&token=' + encodeURIComponent(state.token) : '');
    }

    function updateSnapshotMeta() {
        var meta = $('#snapshot-meta');
        if (!meta) return;
        if (state.snapshotLoadTime === 0) {
            if (state.snapshotFailures > 0) {
                meta.textContent = t('camera.connecting');
                meta.classList.remove('live');
            } else {
                meta.textContent = '—';
                meta.classList.remove('live');
            }
            return;
        }
        var elapsed = Math.floor((Date.now() - state.snapshotLoadTime) / 1000);
        meta.classList.add('live');
        meta.textContent = t('camera.live') + '  ·  ' + elapsed + 's';
    }

    /* ======================================================================
       Camera Tab — Imaging Controls
       ====================================================================== */

    function loadImaging() {
        Promise.all([
            api('GET', '/api/camera/params'),
            api('GET', '/api/camera/options')
        ]).then(function (results) {
            renderImaging(results[0] || {}, results[1] || {});
        }).catch(function (err) {
            if (err.status !== 401) {
                showToast('toast.imagingLoad', { err: err.message });
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

            var wrap = el('div', { className: 'param-control' });

            var header = el('div', { className: 'param-header' });
            header.appendChild(el('span', { className: 'param-label', textContent: label }));
            var valSpan = el('span', { className: 'param-value mono', textContent: formatNumber(current, step) });
            header.appendChild(valSpan);
            wrap.appendChild(header);

            var slider = el('input', {
                className: 'param-slider',
                type: 'range',
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
        wrap.appendChild(el('span', { className: 'param-label', textContent: label }));

        var sel = el('select', { className: 'param-select', 'data-param': name });
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
       Camera Tab — PTZ Controls
       ====================================================================== */

    function loadPTZ() {
        state.ptzRendered = true;
        api('GET', '/api/ptz/status').then(updatePTZDisplay).catch(function () { /* WS will update */ });
        loadPresets();
    }

    function updatePTZDisplay(data) {
        if (!data) return;
        var pos = data.position || data;
        var pan = pos.Pan !== undefined ? pos.Pan : 0;
        var tilt = pos.Tilt !== undefined ? pos.Tilt : 0;
        var zoom = pos.Zoom !== undefined ? pos.Zoom : 0;

        $('#ptz-pan').textContent = formatNumber(pan, 0.001);
        $('#ptz-tilt').textContent = formatNumber(tilt, 0.001);
        $('#ptz-zoom').textContent = formatNumber(zoom, 0.001);

        updatePTZStage(pan, tilt, zoom);

        var statusEl = $('#ptz-status');
        if (statusEl && data.status) {
            var statusKey = data.status === 'MOVING' ? 'ptz.moving' :
                            data.status === 'IDLE' ? 'ptz.idle' : null;
            statusEl.textContent = statusKey ? t(statusKey) : data.status;
            statusEl.className = 'ptz-value ptz-status-badge ' + data.status.toLowerCase();
        }
    }

    function updatePTZStage(pan, tilt, zoom) {
        var dot = $('#ptz-stage-dot');
        if (!dot) return;
        var pad = 10;
        var range = 100 - pad * 2;
        var x = pad + (pan + 1) / 2 * range;
        var y = pad + (1 - (tilt + 1) / 2) * range;
        dot.style.left = x + '%';
        dot.style.top = y + '%';
    }

    function initPTZControls() {
        var dirs = {
            up:    { Pan:  0,   Tilt:  0.5, Zoom: 0 },
            down:  { Pan:  0,   Tilt: -0.5, Zoom: 0 },
            left:  { Pan: -0.5, Tilt:  0,   Zoom: 0 },
            right: { Pan:  0.5, Tilt:  0,   Zoom: 0 }
        };

        $$('.dpad-btn[data-dir]').forEach(function (btn) {
            var dir = btn.dataset.dir;
            if (!dirs[dir]) return;

            btn.addEventListener('mousedown', function (e) { e.preventDefault(); startMove(dirs[dir]); });
            btn.addEventListener('mouseup', stopMove);
            btn.addEventListener('mouseleave', stopMove);
            btn.addEventListener('touchstart', function (e) {
                e.preventDefault(); startMove(dirs[dir]);
            }, { passive: false });
            btn.addEventListener('touchend', stopMove);
            btn.addEventListener('touchcancel', stopMove);
        });

        var stopBtn = $('#btn-ptz-stop');
        if (stopBtn) {
            stopBtn.addEventListener('mousedown', function (e) { e.preventDefault(); });
            stopBtn.addEventListener('click', stopMove);
        }

        var zoomIn = $('[data-dir="zoom-in"]');
        var zoomOut = $('[data-dir="zoom-out"]');

        if (zoomIn) {
            zoomIn.addEventListener('mousedown', function (e) { e.preventDefault(); startMove({ Pan: 0, Tilt: 0, Zoom: 0.3 }); });
            zoomIn.addEventListener('mouseup', stopMove);
            zoomIn.addEventListener('mouseleave', stopMove);
            zoomIn.addEventListener('touchstart', function (e) {
                e.preventDefault(); startMove({ Pan: 0, Tilt: 0, Zoom: 0.3 });
            }, { passive: false });
            zoomIn.addEventListener('touchend', stopMove);
            zoomIn.addEventListener('touchcancel', stopMove);
        }
        if (zoomOut) {
            zoomOut.addEventListener('mousedown', function (e) { e.preventDefault(); startMove({ Pan: 0, Tilt: 0, Zoom: -0.3 }); });
            zoomOut.addEventListener('mouseup', stopMove);
            zoomOut.addEventListener('mouseleave', stopMove);
            zoomOut.addEventListener('touchstart', function (e) {
                e.preventDefault(); startMove({ Pan: 0, Tilt: 0, Zoom: -0.3 });
            }, { passive: false });
            zoomOut.addEventListener('touchend', stopMove);
            zoomOut.addEventListener('touchcancel', stopMove);
        }

        /* Keyboard shortcuts: Arrow keys for PTZ when camera tab is active */
        document.addEventListener('keydown', function (e) {
            if (state.currentTab !== 'camera') return;
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT' || e.target.tagName === 'TEXTAREA') return;
            var keyMap = {
                ArrowUp: dirs.up, ArrowDown: dirs.down,
                ArrowLeft: dirs.left, ArrowRight: dirs.right
            };
            if (keyMap[e.key]) {
                e.preventDefault();
                if (!state._keyMoving) {
                    state._keyMoving = true;
                    startMove(keyMap[e.key]);
                }
            }
            if (e.key === '+' || e.key === '=') {
                e.preventDefault();
                if (!state._keyMoving) {
                    state._keyMoving = true;
                    startMove({ Pan: 0, Tilt: 0, Zoom: 0.3 });
                }
            }
            if (e.key === '-') {
                e.preventDefault();
                if (!state._keyMoving) {
                    state._keyMoving = true;
                    startMove({ Pan: 0, Tilt: 0, Zoom: -0.3 });
                }
            }
        });
        document.addEventListener('keyup', function (e) {
            if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', '+', '=', '-'].indexOf(e.key) !== -1) {
                if (state._keyMoving) {
                    state._keyMoving = false;
                    stopMove();
                }
            }
        });
    }

    function startMove(velocity) {
        api('POST', '/api/ptz/move', velocity).catch(function (err) {
            if (err.status !== 401) {
                showToast('toast.ptzMove', { err: err.message }, { kind: 'error' });
            }
        });
    }

    function stopMove() {
        api('POST', '/api/ptz/stop').catch(function () { /* best effort */ });
    }

    /* ======================================================================
       Camera Tab — Presets
       ====================================================================== */

    function loadPresets() {
        api('GET', '/api/ptz/presets').then(renderPresets).catch(function () {
            renderPresets([]);
        });
    }

    function renderPresets(presets) {
        state.presetsRendered = true;
        var container = $('#preset-list');
        if (!container) return;
        container.innerHTML = '';

        if (!presets || !presets.length) {
            var empty = el('div', { className: 'empty-state', id: 'preset-empty' });
            empty.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="empty-icon"><path d="M21 10c0 7-9 13-9 13S3 17 3 10a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/></svg>';
            empty.appendChild(el('p', { textContent: t('ptz.noPresets') }));
            container.appendChild(empty);
            return;
        }

        presets.forEach(function (p) {
            var row = el('div', { className: 'preset-row' });

            var info = el('div', { className: 'preset-info' });
            info.appendChild(el('span', { className: 'preset-name', textContent: p.name || (t('ptz.presetName') + ' ' + p.token) }));

            var posText = 'Token: ' + p.token;
            if (p.position) {
                var pan = formatNumber(p.position.Pan || 0, 0.01);
                var tilt = formatNumber(p.position.Tilt || 0, 0.01);
                var zoom = formatNumber(p.position.Zoom || 0, 0.01);
                posText = t('ptz.presetPos', { p: pan, t: tilt, z: zoom });
            }
            info.appendChild(el('span', { className: 'preset-pos mono', textContent: posText }));
            row.appendChild(info);

            var actions = el('div', { className: 'preset-actions' });
            actions.appendChild(el('button', {
                className: 'btn btn-sm',
                textContent: t('ptz.goto'),
                onClick: function () { gotoPreset(p.token); }
            }));
            actions.appendChild(el('button', {
                className: 'btn btn-sm btn-danger',
                textContent: t('ptz.delete'),
                onClick: function () { deletePreset(p.token); }
            }));
            row.appendChild(actions);

            container.appendChild(row);
        });
    }

    function addPreset() {
        var count = $$('#preset-list .preset-row').length;
        api('POST', '/api/ptz/preset', { name: t('ptz.presetName') + ' ' + (count + 1) })
            .then(loadPresets)
            .catch(function (err) {
                if (err.status !== 401) {
                    showToast('toast.presetAdd', { err: err.message }, { kind: 'error' });
                }
            });
    }

    function gotoPreset(token) {
        api('POST', '/api/ptz/preset/goto', { token: token }).catch(function (err) {
            if (err.status !== 401) {
                showToast('toast.presetGoto', { err: err.message }, { kind: 'error' });
            }
        });
    }

    function deletePreset(token) {
        var confirmMsg = state.lang === 'zh' ? '确定要删除此预置位吗？' : 'Are you sure you want to delete this preset?';
        showConfirmModal(confirmMsg).then(function () {
            api('DELETE', '/api/ptz/preset/' + encodeURIComponent(token))
                .then(loadPresets)
                .catch(function (err) {
                    if (err.status !== 401) {
                        showToast('toast.presetDelete', { err: err.message }, { kind: 'error' });
                    }
                });
        }).catch(function () { /* cancelled — do nothing */ });
    }

    function initPresetControls() {
        var btn = $('#btn-add-preset');
        if (btn) btn.addEventListener('click', addPreset);
    }

    /* ======================================================================
       WebSocket
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
        /* Clear camera timers on disconnect */
        if (state.snapshotTimer) {
            clearInterval(state.snapshotTimer);
            state.snapshotTimer = null;
        }
        if (state.metaTimer) {
            clearInterval(state.metaTimer);
            state.metaTimer = null;
        }
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
            case 'ptz-position':
                if (msg.position && msg.position.Pan !== undefined) {
                    updatePTZDisplay(msg);
                } else {
                    fetchPTZStatus();
                }
                break;
            case 'preset-list-changed':
                loadPresets();
                break;
        }
    }

    function fetchPTZStatus() {
        api('GET', '/api/ptz/status').then(updatePTZDisplay).catch(function () { /* silent */ });
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
        initPTZControls();
        initPresetControls();
        initLogout();

		fetchVersion();
        loadConfig();
        initSnapshot();
        initLiveVideo();
        loadImaging();
        loadPTZ();

        applyI18n();
        connectWS();

        /* Clean up all timers on page close to prevent memory leaks */
        window.addEventListener('beforeunload', function () {
            if (state.snapshotTimer) {
                clearInterval(state.snapshotTimer);
                state.snapshotTimer = null;
            }
            if (state.metaTimer) {
                clearInterval(state.metaTimer);
                state.metaTimer = null;
            }
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
