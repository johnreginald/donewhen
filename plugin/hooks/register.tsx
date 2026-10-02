import { atom, read, update } from 'claude-code'
import type { Register } from 'claude-code'

import type { Dw, Row } from '../types'

// /dw: a menu for DoneWhen tickets. Every step reads the REST API through
// scripts/tasks.mjs, so browsing makes no model call. "Start work" is the one
// step that submits a prompt.
const PANE = 'dw'

const EMPTY: Dw = {
  view: 'workspaces',
  isLoading: false,
  error: '',
  workspaces: [],
  workspace: '',
  workspaceName: '',
  states: [],
  epics: [],
  issues: [],
  epic: '',
  state: '',
  page: 0,
  isAll: false,
  detail: null,
}

const ui = atom({ plugin: 'donewhen', key: 'ui' } as const, EMPTY)

const PAGE = 15

// Terminal colour per workflow state: the same hues as the web app.
const STATE_COLOR: Record<string, string> = {
  Triage: 'gray',
  Backlog: 'gray',
  Aligning: 'magenta',
  Ready: 'cyan',
  'In Progress': 'yellow',
  Blocked: 'red',
  'In Review': 'yellow',
  Done: 'green',
  Canceled: 'gray',
}
const colorOf = (state: string) => STATE_COLOR[state] ?? 'white'
const bar = (done: number, total: number, width = 20) => {
  const n = total ? Math.round((done / total) * width) : 0
  return '█'.repeat(n) + '░'.repeat(width - n)
}

// The prompt that starts work on a ticket. The model reads it as a normal turn.
const workPrompt = (key: string, title: string, workspace: string) =>
  `Work on DoneWhen ticket ${key} "${title}" (workspace ${workspace}). Read it with get_issue and get_criteria, then use the donewhen skill: build it, tick each done-when item the moment it is met, link the commit, and stop at In Review.`
const ALL = '*'

async function tasks($: any, args: string[]): Promise<any> {
  const script = `${$.plugin.root}/scripts/tasks.mjs`
  let out: any
  try {
    out = await $.process.run(['node', script, ...args], { timeoutMs: 25000 })
  } catch (err) {
    return { error: `Could not start node (${String((err as any)?.message ?? err).slice(0, 120)}). Script: ${script}` }
  }
  try {
    return JSON.parse(out.stdout)
  } catch {
    const text = (out.stdout || out.stderr || '').trim().split('\n')[0] || `exit ${out.exitCode}, no output`
    return { error: text.slice(0, 200) }
  }
}

const patch = ($: any, change: Partial<Dw>) => update($, ui, (s: Dw) => ({ ...s, ...change }))

// loadWorkspaces fills the picker. With goLast, it opens the last workspace
// used (kept across sessions) when that workspace is still reachable.
async function loadWorkspaces($: any, goLast = false) {
  await patch($, { view: 'workspaces', isLoading: true, error: '' })
  const r = await tasks($, ['workspaces', '--json'])
  if (r.error) return patch($, { isLoading: false, error: String(r.error) })
  const list = (r.workspaces ?? []).map((w: any) => ({ slug: w.slug, name: w.name, prefix: w.prefix, open: w.open }))
  await patch($, { isLoading: false, workspaces: list })
  if (goLast) {
    let last: unknown
    try {
      last = await $.store.get('lastWorkspace')
    } catch {}
    const w = list.find((x: any) => x.slug === last)
    if (w) await loadList($, w.slug, w.name, false)
  }
}

async function loadList($: any, slug: string, name: string, isAll: boolean) {
  try {
    await $.store.set('lastWorkspace', slug)
  } catch {}
  await patch($, { view: 'list', isLoading: true, error: '', workspace: slug, workspaceName: name, isAll, page: 0 })
  const r = await tasks($, ['data', slug, ...(isAll ? ['--all'] : [])])
  if (r.error) return patch($, { isLoading: false, error: String(r.error) })
  await patch($, { isLoading: false, states: r.states, epics: r.epics, issues: r.issues })
}

async function loadDetail($: any, slug: string, key: string) {
  await patch($, { view: 'ticket', isLoading: true, error: '', detail: null })
  const r = await tasks($, ['show', slug, key])
  if (r.error) return patch($, { isLoading: false, error: String(r.error) })
  await patch($, { isLoading: false, detail: r })
}

const cut = (s: string, n: number) => (s.length > n ? `${s.slice(0, Math.max(1, n - 1))}…` : s)

function visible(s: Dw): Row[] {
  return s.issues.filter(
    (i) => (!s.epic || i.epic === s.epic) && (!s.state || i.state === s.state),
  )
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'dw',
      description: 'Browse DoneWhen tickets in a menu (no model call)',
    })
    return next(e)
  })

  on('command.run', { command: 'dw' }, async $ => {
    await $.ui.open({ id: PANE, title: 'DoneWhen', focus: true, closeOnEscape: true })
    await update($, ui, () => EMPTY)
    void loadWorkspaces($, true)
    return { text: 'DoneWhen menu opened.' }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Button, Select } = $.ui.resolve(e)
    try {
      return await draw($, e, { Box, Text, Button, Select })
    } catch (err) {
      return <Text color="red">Draw error: {String((err as any)?.message ?? err).slice(0, 160)}</Text>
    }
  })
}

async function draw($: any, e: any, { Box, Text, Button, Select }: any) {
  {
    const s = await read($, ui)
    const cols = (e.props as any)?.bodyColumns ?? e.viewport?.columns ?? 80

    if (s.error) {
      return (
        <Box flexDirection="column">
          <Text color="red">{s.error}</Text>
          <Button label="Back to workspaces" onPress={() => void loadWorkspaces($)} />
        </Box>
      )
    }

    if (s.isLoading) return <Text dimColor>Loading…</Text>

    if (s.view === 'workspaces') {
      if (s.workspaces.length === 0) return <Text dimColor>No workspace found.</Text>
      return (
        <Box flexDirection="column">
          <Text bold>Which workspace?</Text>
          <Select
            key="workspace"
            autoFocus
            options={s.workspaces.map(w => ({
              value: w.slug,
              label: `${w.name} (${w.prefix}) · ${w.open} open`,
            }))}
            onSelect={slug => {
              const w = s.workspaces.find(x => x.slug === slug)
              void loadList($, slug, w?.name ?? slug, false)
            }}
          />
        </Box>
      )
    }

    if (s.view === 'ticket') {
      const d = s.detail
      if (!d) return <Text dimColor>No ticket.</Text>
      const done = d.criteria.filter(c => c.done).length
      return (
        <Box flexDirection="column">
          <Text bold>
            {d.key} · {d.title}
          </Text>
          <Text>
            <Text color={colorOf(d.state)} bold>
              {d.state}
            </Text>
            <Text dimColor>{d.epic ? `  ·  ${d.epic}` : ''}</Text>
          </Text>
          <Box flexDirection="column" marginTop={1}>
            {d.description.slice(0, 3).map((l: string) => (
              <Text dimColor>{cut(l, cols - 2)}</Text>
            ))}
          </Box>
          <Box flexDirection="column" marginTop={1}>
            <Text>
              <Text bold>Done-when </Text>
              <Text color={done === d.criteria.length && done > 0 ? 'green' : 'yellow'}>
                {bar(done, d.criteria.length)} {done}/{d.criteria.length}
              </Text>
            </Text>
            {d.criteria.map((c: { text: string; done: boolean }) => (
              <Text dimColor={c.done}>
                <Text color={c.done ? 'green' : 'yellow'}>{c.done ? '☑' : '☐'}</Text> {cut(c.text, cols - 4)}
              </Text>
            ))}
          </Box>
          {d.branch ? <Text dimColor>branch {d.branch}</Text> : null}
          {d.prUrl ? <Text dimColor>PR {d.prUrl}</Text> : null}
          <Box marginTop={1}>
            <Button
              label="Start work"
              hotkey="g"
              variant="primary"
              onPress={async () => {
                await $.ui.close({ id: PANE })
                void $.prompt.submit({ text: workPrompt(d.key, d.title, s.workspace) })
              }}
            />
            <Button
              label="Put in prompt"
              hotkey="f"
              onPress={async () => {
                await $.prompt.fill({ text: workPrompt(d.key, d.title, s.workspace) })
                await $.ui.close({ id: PANE })
              }}
            />
            <Button label="Back to list" hotkey="b" onPress={() => void patch($, { view: 'list', detail: null })} />
            <Button
              label="Copy link"
              hotkey="c"
              onPress={() => {
                void $.ui.copy({ text: d.link, surface: e.surface })
                void $.ui.toast('Link copied.')
              }}
            />
          </Box>
        </Box>
      )
    }

    // list view: the tickets first, the filters as buttons that step through the values
    const rows = visible(s)
    const pages = Math.max(1, Math.ceil(rows.length / PAGE))
    const page = Math.min(s.page, pages - 1)
    const shown = rows.slice(page * PAGE, page * PAGE + PAGE)
    const room = Math.max(3, cols - 28)
    const next = (names: string[], now: string) => {
      const all = ['', ...names]
      return all[(all.indexOf(now) + 1) % all.length]
    }
    return (
      <Box flexDirection="column">
        <Text bold color="cyan">
          {s.workspaceName}
          <Text dimColor>
            {'  '}
            {rows.length} {s.isAll ? 'tickets' : 'open'} · page {page + 1}/{pages}
            {s.epic ? ` · epic ${s.epic}` : ''}
            {s.state ? ` · ${s.state}` : ''}
          </Text>
        </Text>
        <Text dimColor>
          {s.states.map(x => `${x.name} ${x.open}`).join('  ·  ')}
        </Text>
        {rows.length === 0 ? (
          <Text dimColor>No ticket matches.</Text>
        ) : (
          <Select
            key="ticket"
            autoFocus
            options={shown.map(i => ({
              value: i.key,
              label: `${i.glyph} ${i.key.padEnd(7)} ${i.state.padEnd(11)} ${cut(i.title, room)}${i.blockedBy ? ' ⊘' : ''}`,
            }))}
            onSelect={key => void loadDetail($, s.workspace, key)}
          />
        )}
        <Box>
          <Button label="Prev page" hotkey="p" onPress={() => void patch($, { page: Math.max(0, page - 1) })} />
          <Button label="Next page" hotkey="n" onPress={() => void patch($, { page: Math.min(pages - 1, page + 1) })} />
        </Box>
        <Box>
          <Button
            label={`Epic: ${s.epic || 'all'}`}
            hotkey="e"
            onPress={() => void patch($, { epic: next(s.epics.map(x => x.name), s.epic), page: 0 })}
          />
          <Button
            label={s.state === 'Ready' ? 'Ready only ✓' : 'Ready only'}
            hotkey="r"
            onPress={() => void patch($, { state: s.state === 'Ready' ? '' : 'Ready', page: 0 })}
          />
          <Button
            label={`State: ${s.state || 'all'}`}
            hotkey="s"
            onPress={() => void patch($, { state: next(s.states.map(x => x.name), s.state), page: 0 })}
          />
        </Box>
        <Box>
          <Button
            label={s.isAll ? 'Hide Done and Canceled' : 'Show Done and Canceled'}
            hotkey="d"
            onPress={() => void loadList($, s.workspace, s.workspaceName, !s.isAll)}
          />
          <Button label="Switch workspace" hotkey="w" onPress={() => void loadWorkspaces($)} />
        </Box>
        <Text dimColor>↑↓ move · Enter open · r ready · e epic · s state · d done · n/p page · w workspace · Esc close</Text>
      </Box>
    )
  }
}
