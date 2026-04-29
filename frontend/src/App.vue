<template>
  <el-config-provider>
    <!-- Standalone pages (e.g. /upload) render without the app shell -->
    <router-view v-if="isStandalone" />

    <div v-else class="app-shell">
      <header class="app-header">
        <div class="brand-lockup">
          <div class="brand-mark">
            <el-icon :size="20"><Files /></el-icon>
          </div>
          <div class="brand-copy">
            <p class="brand-eyebrow">Kipup workspace</p>
            <h1 class="brand-title">S3 storage, shaped with calmer typography and steadier rhythm.</h1>
            <p class="brand-subtitle">Quiet S3 management for browsing, collaboration, and sharing.</p>
          </div>
        </div>
        <div class="brand-meta">
          <p>Browse buckets, move files, and share uploads from one quiet control surface.</p>
          <p>Keep bucket browsing, transfers, and upload sharing organized in one focused workspace.</p>
        </div>
      </header>
      <main class="app-main">
        <router-view />
      </main>
    </div>
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
:root {
  --kip-bg: #f4efe6;
  --kip-surface: rgba(255, 252, 245, 0.88);
  --kip-surface-strong: #fffcf5;
  --kip-border: rgba(69, 54, 42, 0.14);
  --kip-border-strong: rgba(69, 54, 42, 0.22);
  --kip-text: #201912;
  --kip-text-muted: #6f6256;
  --kip-accent: #201912;
  --kip-accent-soft: #ede2d2;
  --kip-shadow: 0 24px 80px rgba(59, 43, 31, 0.08);
  --el-color-primary: #201912;
  --el-border-radius-base: 18px;
  --el-border-radius-small: 14px;
  --el-border-color: var(--kip-border);
  --el-border-color-light: rgba(69, 54, 42, 0.08);
  --el-fill-color-light: rgba(237, 226, 210, 0.55);
  --el-fill-color-lighter: rgba(255, 252, 245, 0.92);
  --el-fill-color-blank: rgba(255, 252, 245, 0.92);
  --el-bg-color: rgba(255, 252, 245, 0.92);
  --el-text-color-primary: var(--kip-text);
  --el-text-color-regular: #4f4338;
  --el-text-color-secondary: var(--kip-text-muted);
  --el-mask-color: rgba(31, 22, 15, 0.48);
  --el-box-shadow-light: 0 20px 60px rgba(52, 39, 28, 0.12);
}

html,
body,
#app {
  margin: 0;
  padding: 0;
  height: 100%;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: var(--kip-bg);
  color: var(--kip-text);
}

.app-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 36px;
  padding: 42px 0 30px;
  border-bottom: 1px solid var(--kip-border);
}

.app-shell {
  min-height: 100vh;
  max-width: 1480px;
  margin: 0 auto;
  padding: 0 28px 36px;
  box-sizing: border-box;
}

.brand-lockup {
  display: flex;
  align-items: flex-start;
  gap: 20px;
}

.brand-mark {
  width: 48px;
  height: 48px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--kip-accent);
  color: #f9f3ea;
  box-shadow: 0 16px 32px rgba(32, 25, 18, 0.16);
}

.brand-eyebrow,
.brand-meta p,
.brand-subtitle {
  margin: 0;
  color: var(--kip-text-muted);
}

.brand-eyebrow {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  margin-bottom: 8px;
}

.brand-title {
  margin: 0;
  font-family: Iowan Old Style, Palatino Linotype, Book Antiqua, Georgia, serif;
  max-width: 780px;
  font-size: 42px;
  font-weight: 600;
  letter-spacing: -0.04em;
  line-height: 1.12;
}

.brand-subtitle {
  margin-top: 12px;
  font-size: 16px;
  line-height: 1.7;
}

.brand-meta {
  max-width: 420px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 15px;
  line-height: 1.7;
}

.app-main {
  padding: 0;
}

.el-button {
  min-height: 40px;
  padding: 0 18px;
  border-radius: 999px;
  font-weight: 500;
}

.el-button--primary {
  background: var(--kip-accent);
  border-color: var(--kip-accent);
  color: #f9f3ea;
}

.el-button--default,
.el-button.is-plain {
  background: rgba(255, 252, 245, 0.72);
  border-color: var(--kip-border);
  color: var(--kip-text);
}

.el-card,
.el-dialog,
.el-drawer,
.el-table,
.el-input__wrapper,
.el-textarea__inner,
.el-select__wrapper,
.el-date-editor.el-input__wrapper,
.el-input-number .el-input__wrapper {
  backdrop-filter: blur(18px);
}

.el-dialog,
.el-drawer__body,
.el-table,
.el-input__wrapper,
.el-textarea__inner,
.el-select__wrapper,
.el-date-editor.el-input__wrapper,
.el-input-number .el-input__wrapper {
  background: rgba(255, 252, 245, 0.9);
}

.el-dialog,
.el-drawer {
  border-radius: 28px 28px 0 0;
}

.el-dialog {
  border: 1px solid var(--kip-border);
  box-shadow: var(--kip-shadow);
}

.el-drawer {
  border-left: 1px solid var(--kip-border);
}

.el-dialog__header,
.el-drawer__header {
  margin-right: 0;
  padding: 24px 28px 0;
}

.el-dialog__body,
.el-dialog__footer,
.el-drawer__body {
  padding-left: 28px;
  padding-right: 28px;
}

.el-table {
  --el-table-border-color: rgba(69, 54, 42, 0.08);
  --el-table-header-bg-color: rgba(237, 226, 210, 0.46);
  --el-table-tr-bg-color: transparent;
  --el-table-row-hover-bg-color: rgba(237, 226, 210, 0.32);
  border: 1px solid rgba(69, 54, 42, 0.08);
  border-radius: 24px;
  box-shadow: var(--kip-shadow);
}

.el-table th.el-table__cell {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--kip-text-muted);
}

.el-table td.el-table__cell,
.el-table th.el-table__cell.is-leaf {
  border-bottom: 1px solid rgba(69, 54, 42, 0.08);
}

.el-input__wrapper,
.el-select__wrapper,
.el-date-editor.el-input__wrapper,
.el-input-number .el-input__wrapper {
  min-height: 44px;
  border-radius: 16px;
  border: 1px solid transparent;
  box-shadow: inset 0 0 0 1px rgba(69, 54, 42, 0.1);
}

.el-input__wrapper.is-focus,
.el-select__wrapper.is-focused,
.el-date-editor.el-input__wrapper.is-focus,
.el-input-number .el-input__wrapper.is-focus {
  box-shadow: inset 0 0 0 1px rgba(32, 25, 18, 0.24);
}

.el-progress-bar__outer {
  background: rgba(69, 54, 42, 0.08);
}

.el-tag {
  border-radius: 999px;
}

.el-empty {
  --el-empty-fill-color-0: #f8f1e6;
  --el-empty-fill-color-1: #f0e5d4;
  --el-empty-fill-color-2: #eadcc8;
  --el-empty-fill-color-3: #ddd0bc;
  --el-empty-fill-color-4: #d0c0aa;
  --el-empty-fill-color-5: #c1ae97;
}
</style>
