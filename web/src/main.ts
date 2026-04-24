import { createApp } from 'vue'
import { createPinia } from 'pinia'
import piniaPersist from 'pinia-plugin-persistedstate'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import * as ElementIcons from '@element-plus/icons-vue'

import App from './App.vue'
import router from './router'
import './styles/global.scss'
import { useSiteStore } from './stores/site'

const app = createApp(App)

const pinia = createPinia()
pinia.use(piniaPersist)
app.use(pinia)
app.use(router)
app.use(ElementPlus, { size: 'default', locale: zhCn })

// 把 element icons 作为全局组件注册,模板里可直接 <el-icon><Setting /></el-icon>
for (const [name, comp] of Object.entries(ElementIcons)) {
  app.component(name, comp as never)
}

// 启动即异步拉取站点公开信息(匿名即可,失败静默),
// 用于登录页 / 注册页 / 顶栏统一展示 site.name / logo / 是否允许注册 等。
useSiteStore(pinia).refresh()

app.mount('#app')
