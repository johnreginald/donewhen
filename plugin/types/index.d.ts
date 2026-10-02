export type Workspace = { slug: string; name: string; prefix: string; open: number }
export type Row = {
  key: string
  number: number
  title: string
  state: string
  glyph: string
  epic: string
  priority: number
  blockedBy: number
}
export type Detail = {
  key: string
  title: string
  state: string
  epic: string
  priority: number
  description: string[]
  criteria: { text: string; done: boolean }[]
  branch: string
  prUrl: string
  link: string
}
export type Dw = {
  view: 'workspaces' | 'list' | 'ticket'
  isLoading: boolean
  error: string
  workspaces: Workspace[]
  workspace: string
  workspaceName: string
  states: { name: string; open: number }[]
  epics: { name: string; open: number }[]
  issues: Row[]
  epic: string
  state: string
  page: number
  isAll: boolean
  detail: Detail | null
}

declare module 'claude-code' {
  interface PluginState {
    'donewhen': { ui: Dw }
  }
}
