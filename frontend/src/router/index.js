import { createRouter, createWebHistory } from 'vue-router'
import Browser from '../views/Browser.vue'
import AuthPage from '../views/AuthPage.vue'
import UploadPage from '../views/UploadPage.vue'
import { hasStoredToken, initializeAuth } from '../auth'

const routes = [
  { path: '/', redirect: '/browser' },
  { path: '/auth', component: AuthPage, name: 'auth', meta: { standalone: true, public: true } },
  { path: '/browser', component: Browser, name: 'browser' },
  { path: '/browser/:bucket', component: Browser, name: 'bucket' },
  { path: '/browser/:bucket/:pathMatch(.*)*', component: Browser, name: 'folder' },
  { path: '/upload', component: UploadPage, name: 'upload', meta: { standalone: true, public: true } }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach(async (to) => {
  if (to.meta?.public) return true
  if (!hasStoredToken()) {
    return { name: 'auth', query: { redirect: to.fullPath } }
  }
  await initializeAuth()
  if (!hasStoredToken()) {
    return { name: 'auth', query: { redirect: to.fullPath } }
  }
  return true
})

export default router
