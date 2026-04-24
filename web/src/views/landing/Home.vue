<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useUIStore } from '@/stores/ui'
import { useSiteStore } from '@/stores/site'

const router = useRouter()
const user = useUserStore()
const ui = useUIStore()
const site = useSiteStore()

const siteName = computed(() => site.get('site.name', 'GPT2API'))
const siteLogo = computed(() => site.get('site.logo_url', ''))
const allowRegister = computed(() => site.allowRegister())
const loggedIn = computed(() => user.isLoggedIn)

function goPlay() {
  if (loggedIn.value) router.push('/personal/play')
  else router.push('/login?redirect=/personal/play')
}
function goDashboard() { router.push('/personal/dashboard') }
function goLogin() { router.push('/login') }
function goRegister() { router.push('/register') }
function scrollTop() { window.scrollTo({ top: 0, behavior: 'smooth' }) }

// 滚动监听,nav 加实体背景
const scrolled = ref(false)
onMounted(() => {
  const onScroll = () => { scrolled.value = window.scrollY > 24 }
  window.addEventListener('scroll', onScroll, { passive: true })
  onScroll()
})

// 三张卖点卡,全部围绕"图"
const features = [
  {
    icon: 'MagicStick',
    color: '#409eff',
    title: 'IMG2 正式版直出',
    desc: '全面对齐 <code>picture_v2</code> 正式协议,SSE 够数即返回,60s 短轮询补齐。<b>速度优先 · 不悄悄重试</b>,出错第一时间暴露给调用方。',
  },
  {
    icon: 'Picture',
    color: '#a855f7',
    title: '批量 · 多比例 · 预设',
    desc: '10 种常用宽高比一键切换(21:9 / 16:9 / 4:3 / 1:1 / 9:16 …),<b>N 张批量成图</b>,提示词预设库,浏览器里直接出图。',
  },
  {
    icon: 'Connection',
    color: '#67c23a',
    title: 'OpenAI 零改造接入',
    desc: '<code>/v1/images/generations</code> · <code>/v1/images/edits</code> 原样对齐官方 SDK,<b>切网关只改 base_url</b>,一行代码即可接入。',
  },
]
</script>

<template>
  <div class="landing" :class="{ dark: ui.isDark }">
    <!-- ============= 顶部导航 ============= -->
    <header class="nav" :class="{ scrolled }">
      <div class="nav-inner">
        <a class="logo" @click="scrollTop">
          <img v-if="siteLogo" :src="siteLogo" class="logo-img" alt="logo" />
          <span v-else class="logo-mark">{{ (siteName[0] || 'G').toUpperCase() }}</span>
          <span class="logo-name">{{ siteName }}</span>
        </a>
        <div class="nav-actions">
          <el-button
            link :title="ui.isDark ? '切换到亮色' : '切换到暗色'"
            class="theme-btn" @click="ui.toggleDark()"
          >
            <el-icon :size="18"><component :is="ui.isDark ? 'Sunny' : 'Moon'" /></el-icon>
          </el-button>
          <template v-if="!loggedIn">
            <el-button text class="btn-login" @click="goLogin">登录</el-button>
            <el-button v-if="allowRegister" type="primary" round @click="goRegister">免费注册</el-button>
          </template>
          <template v-else>
            <el-button type="primary" round @click="goDashboard">
              进入控制台 <el-icon><ArrowRight /></el-icon>
            </el-button>
          </template>
        </div>
      </div>
    </header>

    <!-- ============= Hero:只讲 GPT IMAGE2 出图 ============= -->
    <section id="hero" class="hero">
      <div class="hero-bg"></div>
      <div class="hero-inner">
        <div class="hero-text">
          <div class="eyebrow">
            <span class="dot"></span>
            gpt-image-2 · 官方级终稿直出
          </div>
          <h1 class="hero-title">
            <span class="gradient-text">GPT IMAGE2</span><br/>
            一键出高清终稿
          </h1>
          <p class="hero-sub">
            基于 chatgpt.com 逆向的 <b>gpt-image-2</b> 网关<br/>
            <b>IMG2 终稿直出</b> · 多比例 / 批量 N 张 / OpenAI SDK 零改造
          </p>
          <div class="hero-cta">
            <el-button size="large" type="primary" round @click="goPlay">
              <el-icon><VideoPlay /></el-icon> 立即体验在线生图
            </el-button>
          </div>
        </div>

        <div class="hero-preview">
          <div class="preview-glow"></div>
          <div class="preview-frame">
            <div class="frame-bar">
              <span class="dot red"></span>
              <span class="dot yellow"></span>
              <span class="dot green"></span>
              <span class="frame-url">/personal/play · gpt-image-2 终稿直出</span>
            </div>
            <img src="/screenshots/playground-xiaoqiao.png" alt="gpt-image-2 单次调用产出多张高清终稿" />
          </div>
        </div>
      </div>
    </section>

    <!-- ============= 三张卖点卡(全围绕"图") ============= -->
    <section class="section features">
      <div class="feature-grid">
        <div v-for="f in features" :key="f.title" class="feature-card">
          <div class="feature-icon" :style="{ background: f.color + '1A', color: f.color }">
            <el-icon :size="22"><component :is="f.icon" /></el-icon>
          </div>
          <div class="feature-title">{{ f.title }}</div>
          <div class="feature-desc" v-html="f.desc"></div>
        </div>
      </div>
      <div class="features-cta">
        <el-button size="large" type="primary" round @click="goPlay">
          立即开始出图 <el-icon><ArrowRight /></el-icon>
        </el-button>
      </div>
    </section>

    <!-- ============= Footer(极简) ============= -->
    <footer class="footer">
      <div class="footer-inner">
        <span>© {{ new Date().getFullYear() }} {{ siteName }}</span>
      </div>
    </footer>
  </div>
</template>

<style scoped lang="scss">
// ========= 全局色变量(同时适配亮 / 暗) =========
.landing {
  --lp-bg: #ffffff;
  --lp-bg-soft: #f7fbff;
  --lp-text: #1f2330;
  --lp-text-soft: #606266;
  --lp-text-mute: #909399;
  --lp-border: rgba(15, 23, 42, 0.08);
  --lp-card: rgba(255, 255, 255, 0.65);
  --lp-card-solid: #ffffff;
  --lp-nav-bg: rgba(255, 255, 255, 0.72);
  --lp-nav-border: rgba(15, 23, 42, 0.08);

  min-height: 100vh;
  background: var(--lp-bg);
  color: var(--lp-text);
  font-family: -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei',
               'Helvetica Neue', Arial, 'Segoe UI', sans-serif;
  line-height: 1.6;
  display: flex;
  flex-direction: column;
}
.landing.dark {
  --lp-bg: #0b1220;
  --lp-bg-soft: #0f1a2e;
  --lp-text: #e6e9ef;
  --lp-text-soft: #b3b7c3;
  --lp-text-mute: #7a8096;
  --lp-border: rgba(255, 255, 255, 0.08);
  --lp-card: rgba(255, 255, 255, 0.04);
  --lp-card-solid: #111a2b;
  --lp-nav-bg: rgba(15, 22, 38, 0.72);
  --lp-nav-border: rgba(255, 255, 255, 0.07);
}

.gradient-text {
  background: linear-gradient(135deg, #409eff 0%, #a855f7 55%, #67c23a 100%);
  background-size: 200% 200%;
  background-clip: text;
  -webkit-background-clip: text;
  color: transparent;
  animation: grad-flow 6s ease-in-out infinite;
}
@keyframes grad-flow {
  0%, 100% { background-position: 0% 50%; }
  50%      { background-position: 100% 50%; }
}

// ========= 顶部导航 =========
.nav {
  position: sticky;
  top: 0;
  z-index: 50;
  padding: 14px 0;
  background: transparent;
  border-bottom: 1px solid transparent;
  transition: background .25s, border-color .25s, box-shadow .25s, padding .25s;
  backdrop-filter: blur(0);
}
.nav.scrolled {
  background: var(--lp-nav-bg);
  border-bottom-color: var(--lp-nav-border);
  backdrop-filter: saturate(180%) blur(12px);
  -webkit-backdrop-filter: saturate(180%) blur(12px);
  padding: 10px 0;
  box-shadow: 0 4px 24px rgba(15, 23, 42, 0.04);
}
.nav-inner {
  max-width: 1240px;
  margin: 0 auto;
  padding: 0 24px;
  display: flex;
  align-items: center;
  gap: 28px;
}
.logo {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  text-decoration: none;
  color: var(--lp-text);
  .logo-img { width: 32px; height: 32px; border-radius: 8px; object-fit: contain; }
  .logo-mark {
    width: 32px; height: 32px; border-radius: 9px;
    display: inline-flex; align-items: center; justify-content: center;
    color: #fff; font-weight: 800; font-size: 15px;
    background: linear-gradient(135deg, #409eff, #a855f7);
    box-shadow: 0 4px 12px #409eff40;
  }
  .logo-name { font-size: 17px; font-weight: 700; letter-spacing: 0.3px; }
}
.menu {
  display: flex;
  gap: 22px;
  flex: 1;
  a {
    color: var(--lp-text-soft);
    font-size: 14px;
    cursor: pointer;
    text-decoration: none;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    transition: color .2s;
    .ext { opacity: .7; }
  }
  a:hover { color: #409eff; }
}
.nav-actions {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  .theme-btn { padding: 4px 8px; }
  .btn-login { font-weight: 600; }
}

// ========= Hero =========
.hero {
  position: relative;
  overflow: hidden;
  padding: 60px 24px 70px;
}
.hero-bg {
  position: absolute;
  inset: -10% -10% 0 -10%;
  pointer-events: none;
  background:
    radial-gradient(900px 420px at 15% 25%, #a5c9ff66, transparent 60%),
    radial-gradient(800px 420px at 85% 15%, #c084fc55, transparent 60%),
    radial-gradient(600px 400px at 70% 90%, #b1f1b255, transparent 60%);
  z-index: 0;
}
.landing.dark .hero-bg {
  background:
    radial-gradient(900px 420px at 15% 25%, #1b3a6a88, transparent 60%),
    radial-gradient(800px 420px at 85% 15%, #4b2a7066, transparent 60%),
    radial-gradient(600px 400px at 70% 90%, #1c4c2666, transparent 60%);
}
.hero-inner {
  position: relative;
  z-index: 1;
  max-width: 1240px;
  margin: 0 auto;
  display: grid;
  grid-template-columns: 1.1fr 1fr;
  gap: 56px;
  align-items: center;
}
.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-radius: 999px;
  border: 1px solid var(--lp-border);
  background: var(--lp-card);
  backdrop-filter: blur(6px);
  font-size: 12px;
  color: var(--lp-text-soft);
  .dot {
    width: 6px; height: 6px; border-radius: 50%;
    background: #67c23a;
    box-shadow: 0 0 0 4px #67c23a33;
  }
}
.hero-title {
  font-size: clamp(40px, 5.4vw, 64px);
  line-height: 1.1;
  margin: 22px 0 18px;
  font-weight: 800;
  letter-spacing: -0.5px;
}
.hero-sub {
  font-size: 16px;
  color: var(--lp-text-soft);
  margin: 0 0 26px;
  b { color: var(--lp-text); font-weight: 600; }
}
.hero-cta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 24px;
}
.hero-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--lp-text-mute);
  .meta-link {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--lp-text-soft);
    text-decoration: none;
    padding: 4px 10px;
    border-radius: 6px;
    transition: background .2s;
  }
  .meta-link:hover { background: var(--lp-card); color: #409eff; }
  .dot-sep { color: var(--lp-text-mute); }
}

.hero-preview {
  position: relative;
  .preview-glow {
    position: absolute;
    inset: -30px;
    background: radial-gradient(closest-side, #a855f744, transparent 70%);
    filter: blur(20px);
    pointer-events: none;
  }
  .preview-frame {
    position: relative;
    background: var(--lp-card-solid);
    border: 1px solid var(--lp-border);
    border-radius: 14px;
    overflow: hidden;
    box-shadow:
      0 30px 60px -20px rgba(168, 85, 247, 0.35),
      0 20px 40px rgba(15, 23, 42, 0.12);
    transform: perspective(1200px) rotateY(-4deg) rotateX(2deg);
    transition: transform .3s;
  }
  .preview-frame:hover {
    transform: perspective(1200px) rotateY(0) rotateX(0);
  }
  .frame-bar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--lp-border);
    background: var(--lp-bg-soft);
    .dot {
      width: 10px; height: 10px; border-radius: 50%;
      &.red    { background: #ff5f57; }
      &.yellow { background: #febc2e; }
      &.green  { background: #28c840; }
    }
    .frame-url {
      margin-left: 10px;
      font-size: 12px;
      color: var(--lp-text-mute);
    }
  }
  img { display: block; width: 100%; height: auto; }
}

// ========= 三张卖点卡 =========
.section {
  padding: 40px 24px 60px;
  position: relative;
}
.features {
  background:
    radial-gradient(800px 300px at 20% 10%, #409eff11, transparent 60%),
    radial-gradient(800px 300px at 80% 90%, #a855f711, transparent 60%);
}
.feature-grid {
  max-width: 1140px;
  margin: 0 auto;
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 18px;
}
.feature-card {
  background: var(--lp-card-solid);
  border: 1px solid var(--lp-border);
  border-radius: 14px;
  padding: 26px 24px;
  transition: transform .2s, box-shadow .2s, border-color .2s;
}
.feature-card:hover {
  transform: translateY(-4px);
  border-color: transparent;
  box-shadow:
    0 20px 40px -12px rgba(64, 158, 255, 0.22),
    0 8px 24px rgba(15, 23, 42, 0.08);
}
.feature-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 14px;
}
.feature-title {
  font-size: 17px;
  font-weight: 700;
  margin-bottom: 8px;
}
.feature-desc {
  font-size: 14px;
  color: var(--lp-text-soft);
  line-height: 1.75;
  :deep(code) {
    padding: 1px 6px;
    border-radius: 4px;
    background: rgba(64, 158, 255, 0.12);
    color: #409eff;
    font-family: 'JetBrains Mono', Menlo, Consolas, monospace;
    font-size: 12px;
  }
  :deep(b) { color: var(--lp-text); font-weight: 600; }
}
.features-cta {
  text-align: center;
  margin-top: 40px;
}

// ========= Footer =========
.footer {
  margin-top: auto;
  background: var(--lp-bg-soft);
  border-top: 1px solid var(--lp-border);
  padding: 22px 24px;
}
.footer-inner {
  max-width: 1240px;
  margin: 0 auto;
  text-align: center;
  font-size: 12.5px;
  color: var(--lp-text-mute);
  a { color: var(--lp-text-soft); text-decoration: none; margin: 0 2px; }
  a:hover { color: #409eff; }
  .sep { margin: 0 8px; color: var(--lp-border); }
}

// ========= 响应式 =========
@media (max-width: 1100px) {
  .hero-inner { grid-template-columns: 1fr; }
  .hero-preview { order: 2; margin-top: 30px; }
}
@media (max-width: 900px) {
  .feature-grid { grid-template-columns: 1fr; }
}
@media (max-width: 640px) {
  .hero { padding: 36px 20px 48px; }
  .section { padding: 28px 20px 48px; }
  .nav-inner { gap: 12px; }
  .nav-actions .btn-login { display: none; }
  .menu { display: none; }
  .hero-cta .el-button { width: 100%; }
  .footer-inner { line-height: 2; }
}
</style>
