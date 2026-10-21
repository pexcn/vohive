<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useRoute, useRouter } from 'vue-router'
import person from '@material-symbols/svg-400/outlined/person.svg?raw'
import lock from '@material-symbols/svg-400/outlined/lock.svg?raw'
import arrowForward from '@material-symbols/svg-400/outlined/arrow_forward.svg?raw'
import SvgIcon from '../components/SvgIcon.vue'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const form = ref({
  username: '',
  password: ''
})

const loading = ref(false)
const loginError = ref('')

function fieldValue(event: Event) {
  return (event.target as HTMLInputElement).value
}

function submitOnEnter(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  if (!loading.value) {
    (event.currentTarget as HTMLFormElement).requestSubmit()
  }
}

async function handleLogin() {
  if (loading.value) return
  loginError.value = ''
  if (!form.value.username || !form.value.password) {
    loginError.value = '请输入用户名和密码'
    return
  }

  loading.value = true
  try {
    const success = await auth.login(form.value.username, form.value.password)
    if (!success) {
      loginError.value = '登录失败，请检查凭证'
      return
    }
    const q = typeof route.query.redirect === 'string' ? route.query.redirect : ''
    let redirect = q ? decodeURIComponent(q) : ''
    if (!redirect) {
      try {
        redirect = sessionStorage.getItem('post_login_redirect') || ''
      } catch {
        // Ignore sessionStorage read failures.
      }
    }
    if (redirect) {
      try {
        sessionStorage.removeItem('post_login_redirect')
      } catch {
        // Ignore sessionStorage delete failures.
      }
      await router.push(redirect)
    } else {
      await router.push('/')
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="login-page">
    <section class="login-card" aria-labelledby="login-title">
      <header class="login-heading">
        <div class="login-mark" aria-hidden="true">VH</div>
        <h1 id="login-title">登录 VoHive</h1>
        <p>4G 模组管理后台</p>
      </header>

      <form @submit.prevent="handleLogin" @keydown.enter="submitOnEnter" class="login-form" :aria-busy="loading">
        <md-outlined-text-field
          :value="form.username"
          label="用户名"
          name="username"
          autocomplete="username"
          required
          :disabled="loading"
          @input="form.username = fieldValue($event)"
        >
          <SvgIcon :svg="person" slot="leading-icon" />
        </md-outlined-text-field>
        <md-outlined-text-field
          :value="form.password"
          label="密码"
          type="password"
          name="password"
          autocomplete="current-password"
          required
          :disabled="loading"
          @input="form.password = fieldValue($event)"
        >
          <SvgIcon :svg="lock" slot="leading-icon" />
        </md-outlined-text-field>
        <p v-if="loginError" class="login-error" role="alert">{{ loginError }}</p>
        <md-filled-button type="submit" :disabled="loading" :aria-busy="loading" class="login-submit">
          <md-circular-progress v-if="loading" slot="icon" indeterminate aria-hidden="true" />
          <SvgIcon v-else :svg="arrowForward" slot="icon" />
          {{ loading ? '正在登录…' : '登录' }}
        </md-filled-button>
      </form>
    </section>
    <footer class="login-footer">VoHive &copy; 2026</footer>
  </main>
</template>

<style scoped>
.login-page {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 72px 24px 32px;
  overflow: auto;
  background: var(--md-sys-color-surface);
  color: var(--md-sys-color-on-surface);
  font-family: var(--md-ref-typeface-plain);
}

.login-card {
  flex: none;
  width: 100%;
  max-width: 420px;
  margin: auto 0;
  padding: 32px;
  border-radius: 28px;
  background: var(--md-sys-color-surface-container-low);
}

.login-heading {
  margin-bottom: 32px;
}

.login-mark {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  margin-bottom: 24px;
  border-radius: 16px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  font-size: 20px;
  font-weight: 600;
}

.login-heading h1 {
  margin: 0;
  font-size: 28px;
  font-weight: 400;
  line-height: 36px;
}

.login-heading p,
.login-footer {
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
}

.login-heading p {
  margin-top: 8px;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.login-submit {
  width: 100%;
}

md-circular-progress {
  --md-circular-progress-size: 18px;
  --md-circular-progress-active-indicator-color: currentColor;
}

.login-error {
  margin: 0;
  padding: 12px 16px;
  border-radius: 12px;
  background: var(--md-sys-color-error-container);
  color: var(--md-sys-color-on-error-container);
  font-size: 14px;
}

.login-footer {
  flex: none;
  margin-top: 24px;
  font-size: 12px;
}
</style>
