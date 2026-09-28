import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import DeploymentView from '../views/DeploymentView.vue'
import ProjectsView from '../views/ProjectsView.vue'
import LibraryView from '../views/LibraryView.vue'
import HistoryView from '../views/HistoryView.vue'
import ReportsView from '../views/ReportsView.vue'
import ExportView from '../views/ExportView.vue'
import AnalysisView from '../views/AnalysisView.vue'
import SettingsView from '../views/SettingsView.vue'
import AboutView from '../views/AboutView.vue'
import DailySummaryView from '../views/DailySummaryView.vue'
import ActivityView from '../views/ActivityView.vue'
import GitInsightsView from '../views/GitInsightsView.vue'
import MyTeamsView from '../views/MyTeamsView.vue'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/deployment',
  },
  {
    path: '/daily-summary',
    name: 'daily-summary',
    component: DailySummaryView,
    meta: { title: 'Daily Summary' },
  },
  {
    path: '/parson-activity',
    name: 'parson-activity',
    component: ActivityView,
    meta: { title: 'Parson Activity' },
  },
  {
    path: '/git-insights',
    name: 'git-insights',
    component: GitInsightsView,
    meta: { title: 'Git Insights' },
  },
  {
    path: '/my-teams',
    name: 'my-teams',
    component: MyTeamsView,
    meta: { title: 'My Teams' },
  },
  {
    path: '/deployment',
    name: 'quick-deploy',
    component: DeploymentView,
    meta: { title: 'Quick Deploy' },
  },
  {
    path: '/projects',
    name: 'Projects',
    component: ProjectsView,
    meta: { title: 'Projects' },
  },
  {
    path: '/library',
    name: 'Library',
    component: LibraryView,
    meta: { title: 'Library' },
  },
  {
    path: '/history',
    name: 'History',
    component: HistoryView,
    meta: { title: 'History' },
  },
  {
    path: '/reports',
    name: 'Reports',
    component: ReportsView,
    meta: { title: 'Reports' },
  },
  {
    path: '/export',
    name: 'Export',
    component: ExportView,
    meta: { title: 'Export' },
  },
  {
    path: '/analysis',
    name: 'Analysis',
    component: AnalysisView,
    meta: { title: 'Analysis' },
  },
  {
    path: '/settings',
    name: 'Settings',
    component: SettingsView,
    meta: { title: 'Settings' },
  },
  {
    path: '/about',
    name: 'About',
    component: AboutView,
    meta: { title: 'About' },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

router.beforeEach((to, from, next) => {
  const title = to.meta?.title as string | undefined
  if (title) {
    document.title = `${title} - chklst`
  }
  next()
})

export default router
