import { createRouter, createWebHistory } from 'vue-router'
import Browser from '../views/Browser.vue'
import UploadPage from '../views/UploadPage.vue'

const routes = [
  { path: '/', redirect: '/browser' },
  { path: '/browser', component: Browser, name: 'browser' },
  { path: '/browser/:bucket', component: Browser, name: 'bucket' },
  { path: '/browser/:bucket/:pathMatch(.*)*', component: Browser, name: 'folder' },
  { path: '/upload', component: UploadPage, name: 'upload', meta: { standalone: true } },
  { path: '/share', component: UploadPage, name: 'share', meta: { standalone: true } }
]

export default createRouter({
  history: createWebHistory(),
  routes
})
