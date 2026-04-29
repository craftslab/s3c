<template>
  <el-config-provider>
    <!-- Standalone pages (e.g. /upload) render without the app shell -->
    <router-view v-if="isStandalone" />

    <el-container v-else class="app-layout">
      <el-header class="app-header">
        <div class="logo">
          <el-icon :size="22"><Files /></el-icon>
          <span>Kipup – Efficient S3 Storage & Download</span>
        </div>
      </el-header>
      <el-main class="app-main">
        <router-view />
      </el-main>
    </el-container>
  </el-config-provider>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Files } from '@element-plus/icons-vue'

const route = useRoute()
const isStandalone = computed(() => !!route.meta?.standalone)
</script>

<style>
html,
body,
#app {
  margin: 0;
  padding: 0;
  height: 100%;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

.app-layout {
  height: 100vh;
}

.app-header {
  display: flex;
  align-items: center;
  background: #409eff;
  color: #fff;
  padding: 0 20px;
}

.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 18px;
  font-weight: 600;
}

.app-main {
  padding: 0;
  overflow: hidden;
}
</style>
