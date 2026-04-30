<template>
  <div class="auth-page">
    <div class="auth-card">
      <div class="auth-copy">
        <p class="auth-eyebrow">Kipup access</p>
        <h1 class="auth-title">Sign in or create a user account.</h1>
        <p class="auth-subtitle">Admins can manage users and permissions after signing in.</p>
      </div>

      <el-tabs v-model="activeTab" stretch>
        <el-tab-pane label="Sign in" name="signin">
          <el-form :model="signInForm" label-position="top" @submit.prevent="submitSignIn">
            <el-form-item label="Username">
              <el-input v-model="signInForm.username" autocomplete="username" />
            </el-form-item>
            <el-form-item label="Password">
              <el-input v-model="signInForm.password" type="password" show-password autocomplete="current-password" />
            </el-form-item>
            <el-button type="primary" :loading="loading" class="auth-submit" @click="submitSignIn">Sign in</el-button>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="Sign up" name="signup">
          <el-form :model="signUpForm" label-position="top" @submit.prevent="submitSignUp">
            <el-form-item label="Username">
              <el-input v-model="signUpForm.username" autocomplete="username" />
            </el-form-item>
            <el-form-item label="Password">
              <el-input v-model="signUpForm.password" type="password" show-password autocomplete="new-password" />
            </el-form-item>
            <el-button type="primary" :loading="loading" class="auth-submit" @click="submitSignUp">Create account</el-button>
          </el-form>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { signIn, signUp } from '../api'
import { setAuthSession } from '../auth'

const route = useRoute()
const router = useRouter()

const activeTab = ref('signin')
const loading = ref(false)
const signInForm = ref({ username: '', password: '' })
const signUpForm = ref({ username: '', password: '' })

async function submitSignIn() {
  if (!signInForm.value.username.trim() || !signInForm.value.password) {
    return ElMessage.warning('Username and password are required')
  }
  loading.value = true
  try {
    const { data } = await signIn({
      username: signInForm.value.username,
      password: signInForm.value.password
    })
    setAuthSession(data.token, data.user)
    ElMessage.success(`Welcome back, ${data.user.username}`)
    await router.replace(resolveRedirect())
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  } finally {
    loading.value = false
  }
}

async function submitSignUp() {
  if (!signUpForm.value.username.trim() || !signUpForm.value.password) {
    return ElMessage.warning('Username and password are required')
  }
  loading.value = true
  try {
    await signUp({
      username: signUpForm.value.username,
      password: signUpForm.value.password
    })
    const { data } = await signIn({
      username: signUpForm.value.username,
      password: signUpForm.value.password
    })
    setAuthSession(data.token, data.user)
    ElMessage.success(`Account created for ${data.user.username}`)
    await router.replace(resolveRedirect())
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  } finally {
    loading.value = false
  }
}

function resolveRedirect() {
  const redirect = route.query.redirect
  if (typeof redirect === 'string' && redirect.startsWith('/')) {
    return redirect
  }
  return { name: 'browser' }
}
</script>

<style scoped>
.auth-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  background:
    radial-gradient(circle at top, rgba(237, 226, 210, 0.82), transparent 42%),
    #f4efe6;
}

.auth-card {
  width: 100%;
  max-width: 520px;
  padding: 40px;
  border-radius: 28px;
  background: rgba(255, 252, 245, 0.92);
  border: 1px solid rgba(69, 54, 42, 0.14);
  box-shadow: 0 24px 80px rgba(59, 43, 31, 0.08);
}

.auth-copy {
  margin-bottom: 20px;
}

.auth-eyebrow,
.auth-subtitle {
  margin: 0;
  color: #6f6256;
}

.auth-eyebrow {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.auth-title {
  margin: 10px 0 8px;
  font-family: Iowan Old Style, Palatino Linotype, Book Antiqua, Georgia, serif;
  font-size: 34px;
  font-weight: 600;
  letter-spacing: -0.04em;
  color: #201912;
}

.auth-subtitle {
  font-size: 15px;
  line-height: 1.7;
}

.auth-submit {
  width: 100%;
}
</style>
