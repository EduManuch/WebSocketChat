/**
 * Auth Module - Login, Register, and Client-side Routing
 */

const Auth = (function() {
    'use strict';

    // Storage keys
    const TOKEN_KEY = 'auth_token';
    const USER_KEY = 'auth_user';

    // API endpoints
    const API_ENDPOINTS = {
        login: '/auth/login',
        register: '/auth/register',
        logout: '/auth/logout',
        me: '/auth/me'
    };

    // Current authenticated user
    let currentUser = null;

    /**
     * Initialize router for client-side navigation
     */
    function initRouter() {
        // Handle browser back/forward buttons
        window.addEventListener('popstate', handlePopState);
        
        // Intercept all links with data-router attribute
        document.addEventListener('click', function(e) {
            const link = e.target.closest('a[data-router]');
            if (link) {
                e.preventDefault();
                const href = link.getAttribute('href');
                navigateTo(href);
            }
        });

        // Handle initial page load
        handleInitialLoad();
    }

    /**
     * Handle initial page load routing
     */
    function handleInitialLoad() {
        const path = window.location.pathname;

        // If on login/register page but already authenticated, redirect to chat
        if (path === '/auth/login' || path === '/auth/register') {
            if (isAuthenticated()) {
                navigateTo('/', true);
                return;
            }
        }

        // If on main page and not authenticated, redirect to login
        if (path === '/') {
            if (!isAuthenticated()) {
                navigateTo('/auth/login', true);
                return;
            }
        }
    }

    /**
     * Handle browser navigation
     */
    function handlePopState(event) {
        if (event.state && event.state.page) {
            renderPage(event.state.page);
        }
    }

    /**
     * Navigate to a new page
     */
    function navigateTo(path, replace = false) {
        if (replace) {
            history.replaceState({ page: path }, '', path);
        } else {
            history.pushState({ page: path }, '', path);
        }
        renderPage(path);
    }

    /**
     * Render page based on path
     */
    function renderPage(path) {
        // For SPA routing, you would fetch and render content here
        // For now, we'll do a full page load (backend handles templates)
        window.location.href = path;
    }

    /**
     * Check if user is authenticated
     */
    function isAuthenticated() {
        const token = localStorage.getItem(TOKEN_KEY);
        return !!token;
    }

    /**
     * Get current user
     */
    function getCurrentUser() {
        if (currentUser) {
            return currentUser;
        }
        
        const userStr = localStorage.getItem(USER_KEY);
        if (userStr) {
            currentUser = JSON.parse(userStr);
        }
        return currentUser;
    }

    /**
     * Get auth token
     */
    function getToken() {
        return localStorage.getItem(TOKEN_KEY);
    }

    /**
     * Check auth status from server
     */
    async function checkAuthStatus() {
        const token = getToken();
        if (!token) {
            return null;
        }

        try {
            const response = await fetch(API_ENDPOINTS.me, {
                method: 'GET',
                headers: {
                    'Authorization': `Bearer ${token}`,
                    'Content-Type': 'application/json'
                }
            });

            if (response.ok) {
                const data = await response.json();
                currentUser = data.user;
                return currentUser;
            } else {
                // Token invalid, clear auth
                clearAuth();
            }
        } catch (error) {
            console.error('Auth check failed:', error);
        }
        
        return null;
    }

    /**
     * Login user
     */
    async function login(username, password) {
        try {
            const response = await fetch(API_ENDPOINTS.login, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ username, password })
            });

            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.message || 'Login failed');
            }

            // Store token and user
            localStorage.setItem(TOKEN_KEY, data.token);
            if (data.user) {
                localStorage.setItem(USER_KEY, JSON.stringify(data.user));
                currentUser = data.user;
            }

            return { success: true, data };
        } catch (error) {
            return { success: false, error: error.message };
        }
    }

    /**
     * Register new user
     */
    async function register(username, email, password) {
        try {
            const response = await fetch(API_ENDPOINTS.register, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ username, email, password })
            });

            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.message || 'Registration failed');
            }

            return { success: true, data };
        } catch (error) {
            return { success: false, error: error.message };
        }
    }

    /**
     * Logout user
     */
    async function logout() {
        const token = getToken();

        try {
            await fetch(API_ENDPOINTS.logout, {
                method: 'POST',
                headers: {
                    'Authorization': `Bearer ${token}`,
                    'Content-Type': 'application/json'
                }
            });
        } catch (error) {
            console.error('Logout request failed:', error);
        } finally {
            clearAuth();
            navigateTo('/auth/login');
        }
    }

    /**
     * Clear authentication data
     */
    function clearAuth() {
        localStorage.removeItem(TOKEN_KEY);
        localStorage.removeItem(USER_KEY);
        currentUser = null;
    }

    /**
     * Validate form fields
     */
    function validateForm(form) {
        let isValid = true;
        const inputs = form.querySelectorAll('input[required]');
        
        inputs.forEach(input => {
            if (!input.value.trim()) {
                input.classList.add('is-invalid');
                input.classList.remove('is-valid');
                isValid = false;
            } else {
                input.classList.remove('is-invalid');
                input.classList.add('is-valid');
            }
        });

        return isValid;
    }

    /**
     * Clear form validation
     */
    function clearValidation(form) {
        const inputs = form.querySelectorAll('input');
        inputs.forEach(input => {
            input.classList.remove('is-invalid', 'is-valid');
        });
    }

    /**
     * Show error message
     */
    function showError(elementId, message) {
        const errorEl = document.getElementById(elementId);
        if (errorEl) {
            errorEl.textContent = message;
            errorEl.classList.remove('d-none');
        }
    }

    /**
     * Hide error message
     */
    function hideError(elementId) {
        const errorEl = document.getElementById(elementId);
        if (errorEl) {
            errorEl.classList.add('d-none');
        }
    }

    /**
     * Set loading state on button
     */
    function setLoading(button, isLoading) {
        const spinner = button.querySelector('.spinner-border');
        
        if (isLoading) {
            button.disabled = true;
            if (spinner) spinner.classList.remove('d-none');
        } else {
            button.disabled = false;
            if (spinner) spinner.classList.add('d-none');
        }
    }

    /**
     * Initialize login form
     */
    function initLogin() {
        const form = document.getElementById('loginForm');
        if (!form) return;

        // Clear any existing errors
        hideError('loginError');
        clearValidation(form);

        form.addEventListener('submit', async function(e) {
            e.preventDefault();

            // Validate form
            if (!validateForm(form)) {
                return;
            }

            const username = document.getElementById('loginUsername').value.trim();
            const password = document.getElementById('loginPassword').value;
            const submitBtn = form.querySelector('button[type="submit"]');

            // Clear previous errors
            hideError('loginError');
            setLoading(submitBtn, true);

            // Call login API
            const result = await login(username, password);

            setLoading(submitBtn, false);

            if (result.success) {
                // Redirect to main page
                navigateTo('/', true);
            } else {
                showError('loginError', result.error);
            }
        });

        // Real-time validation
        form.querySelectorAll('input').forEach(input => {
            input.addEventListener('blur', function() {
                if (this.value.trim()) {
                    this.classList.remove('is-invalid');
                    this.classList.add('is-valid');
                }
            });
            
            input.addEventListener('input', function() {
                this.classList.remove('is-invalid');
                hideError('loginError');
            });
        });
    }

    /**
     * Initialize register form
     */
    function initRegister() {
        const form = document.getElementById('registerForm');
        if (!form) return;

        // Clear any existing errors/success messages
        hideError('registerError');
        hideError('registerSuccess');
        clearValidation(form);

        form.addEventListener('submit', async function(e) {
            e.preventDefault();

            // Validate form
            if (!validateForm(form)) {
                return;
            }

            // Additional validation
            const password = document.getElementById('registerPassword').value;
            const passwordConfirm = document.getElementById('registerPasswordConfirm').value;
            
            if (password !== passwordConfirm) {
                const confirmInput = document.getElementById('registerPasswordConfirm');
                confirmInput.classList.add('is-invalid');
                confirmInput.classList.remove('is-valid');
                showError('registerError', 'Passwords do not match');
                return;
            }

            const username = document.getElementById('registerUsername').value.trim();
            const email = document.getElementById('registerEmail').value.trim();
            const submitBtn = form.querySelector('button[type="submit"]');

            // Clear previous messages
            hideError('registerError');
            hideError('registerSuccess');
            setLoading(submitBtn, true);

            // Call register API
            const result = await register(username, email, password);

            setLoading(submitBtn, false);

            if (result.success) {
                // Show success message and redirect to login
                const successEl = document.getElementById('registerSuccess');
                successEl.textContent = 'Account created successfully! Redirecting to login...';
                successEl.classList.remove('d-none');

                setTimeout(() => {
                    navigateTo('/auth/login');
                }, 1500);
            } else {
                showError('registerError', result.error);
            }
        });

        // Real-time validation
        form.querySelectorAll('input').forEach(input => {
            input.addEventListener('blur', function() {
                if (this.value.trim() || !this.hasAttribute('required')) {
                    // Check password match on blur
                    if (this.id === 'registerPasswordConfirm') {
                        const password = document.getElementById('registerPassword').value;
                        if (this.value === password && this.value) {
                            this.classList.remove('is-invalid');
                            this.classList.add('is-valid');
                        }
                    } else if (this.value.trim()) {
                        this.classList.remove('is-invalid');
                        this.classList.add('is-valid');
                    }
                }
            });
            
            input.addEventListener('input', function() {
                this.classList.remove('is-invalid');
                hideError('registerError');
                
                // Real-time password match check
                if (this.id === 'registerPassword' || this.id === 'registerPasswordConfirm') {
                    const password = document.getElementById('registerPassword').value;
                    const confirm = document.getElementById('registerPasswordConfirm').value;
                    const confirmInput = document.getElementById('registerPasswordConfirm');
                    
                    if (confirm && password === confirm) {
                        confirmInput.classList.remove('is-invalid');
                        confirmInput.classList.add('is-valid');
                    }
                }
            });
        });
    }

    /**
     * Initialize auth module
     */
    function init() {
        initRouter();
    }

    // Public API
    return {
        init: init,
        initLogin: initLogin,
        initRegister: initRegister,
        login: login,
        register: register,
        logout: logout,
        isAuthenticated: isAuthenticated,
        getCurrentUser: getCurrentUser,
        getToken: getToken,
        navigateTo: navigateTo
    };

})();

// Auto-initialize on DOM ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function() {
        Auth.init();
    });
} else {
    Auth.init();
}
