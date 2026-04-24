<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useUserStore } from '@/stores/user'
import { useUIStore } from '@/stores/ui'
import { useSiteStore } from '@/stores/site'
import { APP_VERSION } from '@/version'
import type { MenuItem } from '@/api/auth'

const store = useUserStore()
const ui = useUIStore()
const site = useSiteStore()
const router = useRouter()
const route = useRoute()

const siteName = computed(() => site.get('site.name', 'GPT2API'))
const siteLogo = computed(() => site.get('site.logo_url', ''))
const siteFooter = computed(() => site.get('site.footer', ''))

const { menu, user, role, permissions } = storeToRefs(store)
const collapsed = ref(false)
const loadingMenu = ref(false)

const activePath = computed(() => route.path)

// 生成面包屑用的路径映射
const titleMap = computed(() => {
  const m = new Map<string, string>()
  function walk(items: MenuItem[]) {
    for (const it of items) {
      if (it.path) m.set(it.path, it.title)
      if (it.children) walk(it.children)
    }
  }
  walk(menu.value)
  return m
})

const currentTitle = computed(() => titleMap.value.get(activePath.value) || (route.meta.title as string) || '')

async function loadMenu() {
  if (menu.value.length > 0) return
  loadingMenu.value = true
  try {
    await store.fetchMenu()
  } finally {
    loadingMenu.value = false
  }
}

async function logout() {
  await store.logout()
  router.replace('/login')
}

function goto(path?: string) {
  if (path) router.push(path)
}

onMounted(loadMenu)
watch(() => store.isLoggedIn, (v) => { if (v) loadMenu() })
</script>

<template>
  <el-container class="layout-root">
    <el-aside :width="collapsed ? '64px' : '220px'" class="sidebar">
      <div class="logo">
        <img v-if="siteLogo" :src="siteLogo" class="logo-img" alt="logo" />
        <span v-else class="mark">{{ (siteName[0] || 'G').toUpperCase() }}</span>
        <span v-if="!collapsed" class="title">{{ siteName }}</span>
      </div>
      <el-menu
        :default-active="activePath"
        :collapse="collapsed"
        background-color="transparent"
        text-color="#cfd3dc"
        active-text-color="#ffffff"
        class="side-menu"
        router
      >
        <template v-for="group in menu" :key="group.key">
          <!-- 无子节点:直接渲染成一级 item -->
          <el-menu-item v-if="!group.children?.length && group.path" :index="group.path">
            <el-icon v-if="group.icon"><component :is="group.icon" /></el-icon>
            <template #title>{{ group.title }}</template>
          </el-menu-item>
          <!-- 有子节点:分组 -->
          <el-sub-menu v-else-if="group.children?.length" :index="group.key">
            <template #title>
              <el-icon v-if="group.icon"><component :is="group.icon" /></el-icon>
              <span>{{ group.title }}</span>
            </template>
            <el-menu-item
              v-for="child in group.children"
              :key="child.key"
              :index="child.path!"
            >
              <el-icon v-if="child.icon"><component :is="child.icon" /></el-icon>
              <template #title>{{ child.title }}</template>
            </el-menu-item>
          </el-sub-menu>
        </template>
      </el-menu>

      <!-- 版本号 -->
      <div class="sidebar-version" :class="{ collapsed }">
        <span class="ver-text">{{ collapsed ? APP_VERSION.replace('v','') : APP_VERSION }}</span>
      </div>
    </el-aside>

    <el-container>
      <el-header class="topbar">
        <div class="left">
          <el-button link @click="collapsed = !collapsed">
            <el-icon :size="18"><component :is="collapsed ? 'Expand' : 'Fold'" /></el-icon>
          </el-button>
          <span class="crumb">{{ currentTitle }}</span>
        </div>
        <div class="right">
          <el-tooltip :content="ui.isDark ? '切换到亮色' : '切换到暗色'" placement="bottom">
            <el-button link class="theme-btn" @click="ui.toggleDark()">
              <el-icon :size="18">
                <component :is="ui.isDark ? 'Sunny' : 'Moon'" />
              </el-icon>
            </el-button>
          </el-tooltip>
          <el-dropdown trigger="click" @command="(c: string) => c === 'logout' ? logout() : goto(c)">
            <span class="user-entry">
              <el-avatar :size="28" style="background:#409eff">
                {{ (user?.nickname || user?.email || 'U').slice(0, 1).toUpperCase() }}
              </el-avatar>
              <span class="nick">{{ user?.nickname || user?.email }}</span>
              <el-tag v-if="role === 'admin'" type="warning" size="small">管理员</el-tag>
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="/personal/dashboard">
                  <el-icon><User /></el-icon> 个人中心
                </el-dropdown-item>
                <el-dropdown-item command="/personal/billing">
                  <el-icon><Wallet /></el-icon> 账单
                </el-dropdown-item>
                <el-dropdown-item divided command="logout">
                  <el-icon><SwitchButton /></el-icon> 退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="main" v-loading="loadingMenu">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>

      <el-footer v-if="siteFooter" class="footer">
        <div class="footer-line footer-custom">{{ siteFooter }}</div>
      </el-footer>
    </el-container>
  </el-container>
</template>

<style scoped lang="scss">
.layout-root { height: 100vh; }

.sidebar {
  background: var(--gp-sidebar-bg);
  transition: width .2s;
  overflow-x: hidden;
  display: flex !important;
  flex-direction: column;
  .side-menu {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
  }
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  color: #fff;
  font-weight: 700;
  letter-spacing: 1px;
  .logo-img {
    width: 32px; height: 32px; border-radius: 8px; object-fit: contain; background: #fff;
  }
  .mark {
    display: inline-flex;
    width: 32px;
    height: 32px;
    border-radius: 8px;
    background: linear-gradient(135deg,#409eff,#67c23a);
    align-items: center; justify-content: center;
    font-size: 14px;
  }
  .title { font-size: 16px; }
}

.side-menu {
  border-right: none !important;
  --el-menu-hover-bg-color: rgba(255,255,255,0.06);
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 56px;
  background: var(--el-bg-color);
  color: var(--el-text-color-primary);
  border-bottom: 1px solid var(--el-border-color-light);
  padding: 0 18px;
  .left { display: flex; align-items: center; gap: 12px; }
  .crumb { font-size: 16px; font-weight: 600; }
  .user-entry {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    color: var(--el-text-color-primary);
    .nick { font-size: 14px; }
  }
  .right {
    display: inline-flex;
    align-items: center;
    gap: 12px;
  }
  .theme-btn { padding: 0 6px; }
}

.main {
  background: var(--gp-bg);
  padding: 0;
}

.footer {
  background: transparent;
  text-align: center;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  padding: 6px 12px;
  height: auto;
  min-height: 36px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
}
.footer-line { line-height: 1.6; }
.brand-line .brand-name {
  color: var(--el-color-primary);
  letter-spacing: 0.5px;
  margin-right: 4px;
}
.brand-line .sep {
  color: var(--el-text-color-disabled);
  margin: 0 4px;
  user-select: none;
}
.footer-custom {
  color: var(--el-text-color-placeholder);
  font-size: 11px;
}
.footer-link {
  color: var(--el-color-primary);
  text-decoration: none;
  margin: 0 2px;
}
.footer-link.pic-link { color: var(--el-color-success); }
.footer-link:hover { text-decoration: underline; }

.fade-enter-active, .fade-leave-active { transition: opacity .15s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

.sidebar-version {
  position: sticky;
  bottom: 0;
  padding: 10px 16px;
  text-align: center;
  border-top: 1px solid rgba(255,255,255,0.07);
  background: var(--gp-sidebar-bg);
  .ver-text {
    display: inline-block;
    font-size: 11px;
    color: rgba(255,255,255,0.28);
    letter-spacing: 0.5px;
    user-select: none;
    white-space: nowrap;
  }
  &.collapsed .ver-text {
    font-size: 9px;
    letter-spacing: 0;
  }
}
</style>
