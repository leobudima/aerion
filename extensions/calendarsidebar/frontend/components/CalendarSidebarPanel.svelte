<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Icon from '@iconify/svelte'
  import { isExtensionEnabled, openExtensionSettings } from '$lib/stores/extensionRegistry.svelte'
  import { setActiveExtension } from '$lib/stores/uiState.svelte'
  // @ts-ignore - wailsjs bindings
  import { Calendar_ListSources, Calendar_ListCalendars, Calendar_ListEventsInRange, Calendar_GetEvent } from '$wailsjs/go/app/App.js'
  // @ts-ignore - wailsjs bindings
  import { EventsOn, EventsOff } from '$wailsjs/runtime/runtime.js'
  // @ts-ignore - wailsjs bindings
  import type { backend } from '$wailsjs/go/models'

  interface Props {
    onClose?: () => void
  }

  let { onClose }: Props = $props()

  // Agenda window: from the start of today to +30 days.
  const AGENDA_DAYS = 30

  let instances = $state<backend.EventInstance[]>([])
  let calendarColors = $state<Record<string, string>>({})
  let loading = $state(false)
  let error = $state<string | null>(null)
  let selectedEvent = $state<backend.Event | null>(null)
  let detailLoading = $state(false)

  const calendarEnabled = $derived(isExtensionEnabled('calendar'))

  type DayGroup = {
    key: string
    label: string
    items: backend.EventInstance[]
  }

  const dayFmt = new Intl.DateTimeFormat(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
  const dateTimeFmt = new Intl.DateTimeFormat(undefined, { weekday: 'short', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  const timeFmt = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' })

  function dateKey(date: Date): string {
    return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`
  }

  const dayGroups = $derived((() => {
    const groups: DayGroup[] = []
    const byKey = new Map<string, DayGroup>()
    for (const inst of instances) {
      const start = new Date(inst.instanceStartUnix * 1000)
      const key = dateKey(start)
      let group = byKey.get(key)
      if (!group) {
        group = { key, label: dayLabel(start), items: [] }
        byKey.set(key, group)
        groups.push(group)
      }
      group.items.push(inst)
    }
    return groups
  })())

  function dayLabel(date: Date): string {
    const today = new Date()
    if (dateKey(date) === dateKey(today)) return 'Today'
    const tomorrow = new Date(today.getTime() + 24 * 60 * 60 * 1000)
    if (dateKey(date) === dateKey(tomorrow)) return 'Tomorrow'
    return dayFmt.format(date)
  }

  function timeLabel(inst: backend.EventInstance): string {
    if (inst.isAllDay) return 'All day'
    const start = new Date(inst.instanceStartUnix * 1000)
    const end = new Date(inst.instanceEndUnix * 1000)
    return `${timeFmt.format(start)} – ${timeFmt.format(end)}`
  }

  async function loadEvents() {
    if (loading) return
    loading = true
    error = null
    try {
      const sources = (await Calendar_ListSources()) || []
      const calendarLists = await Promise.all(
        sources.filter(s => s.enabled).map(s => Calendar_ListCalendars(s.id))
      )
      const visible = calendarLists.flat().filter(c => c && c.visible)
      const colors: Record<string, string> = {}
      for (const c of visible) colors[c.id] = c.color || ''
      calendarColors = colors

      if (visible.length === 0) {
        instances = []
        return
      }

      const from = new Date()
      from.setHours(0, 0, 0, 0)
      const to = new Date(from.getTime() + AGENDA_DAYS * 24 * 60 * 60 * 1000)
      instances = (await Calendar_ListEventsInRange(
        visible.map(c => c.id),
        Math.floor(from.getTime() / 1000),
        Math.floor(to.getTime() / 1000),
      )) || []
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
      instances = []
    } finally {
      loading = false
    }
  }

  async function openDetail(inst: backend.EventInstance) {
    detailLoading = true
    try {
      selectedEvent = await Calendar_GetEvent(inst.id)
    } catch {
      // Fall back to the instance data we already have
      selectedEvent = inst
    } finally {
      detailLoading = false
    }
  }

  onMount(() => {
    if (calendarEnabled) void loadEvents()
    EventsOn('calendar:sync-complete', () => {
      if (calendarEnabled) void loadEvents()
    })
  })

  onDestroy(() => {
    EventsOff('calendar:sync-complete')
  })

  // Re-load when the calendar extension gets enabled while the panel is open.
  $effect(() => {
    if (calendarEnabled) void loadEvents()
  })
</script>

<div class="flex flex-col h-full border-l border-border bg-background">
  <!-- Header -->
  <div class="flex items-center gap-2 px-3 h-10 border-b border-border flex-shrink-0">
    <Icon icon="mdi:calendar-month-outline" class="w-4 h-4 text-muted-foreground" />
    <span class="text-sm font-medium flex-1 truncate">Upcoming</span>
    <button
      type="button"
      class="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
      title="Open full calendar"
      onclick={() => setActiveExtension('calendar')}
    >
      <Icon icon="mdi:arrow-expand" class="w-4 h-4" />
    </button>
    <button
      type="button"
      class="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
      title="Refresh"
      onclick={() => loadEvents()}
      disabled={loading}
    >
      <Icon icon="mdi:refresh" class="w-4 h-4 {loading ? 'animate-spin' : ''}" />
    </button>
    <button
      type="button"
      class="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
      title="Close"
      onclick={() => onClose?.()}
    >
      <Icon icon="mdi:close" class="w-4 h-4" />
    </button>
  </div>

  {#if !calendarEnabled}
    <!-- The panel is data-less without the Calendar extension -->
    <div class="flex flex-col items-center justify-center flex-1 gap-2 p-4 text-center text-sm text-muted-foreground">
      <Icon icon="mdi:puzzle-outline" class="w-8 h-8" />
      <p>The Calendar Sidebar needs the Calendar extension for its data.</p>
      <button
        type="button"
        class="text-primary hover:underline"
        onclick={() => openExtensionSettings('calendar')}
      >
        Enable the Calendar extension
      </button>
    </div>
  {:else if selectedEvent}
    <!-- Event detail -->
    <div class="flex-1 overflow-y-auto">
      <div class="flex items-center gap-1 px-2 py-1.5 border-b border-border">
        <button
          type="button"
          class="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
          title="Back"
          onclick={() => selectedEvent = null}
        >
          <Icon icon="mdi:arrow-left" class="w-4 h-4" />
        </button>
        <span class="text-xs text-muted-foreground">Event details</span>
      </div>
      <div class="p-3 space-y-3">
        <h3 class="text-sm font-semibold leading-snug">{selectedEvent.summary || '(no title)'}</h3>
        <div class="flex items-start gap-2 text-xs text-muted-foreground">
          <Icon icon="mdi:clock-outline" class="w-4 h-4 flex-shrink-0 mt-0.5" />
          <span>
            {#if selectedEvent.isAllDay}
              {dayFmt.format(new Date(selectedEvent.dtstartUnix * 1000))} · All day
            {:else}
              {dateTimeFmt.format(new Date(selectedEvent.dtstartUnix * 1000))} – {timeFmt.format(new Date(selectedEvent.dtendUnix * 1000))}
            {/if}
          </span>
        </div>
        {#if selectedEvent.location}
          <div class="flex items-start gap-2 text-xs text-muted-foreground">
            <Icon icon="mdi:map-marker-outline" class="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span class="break-words">{selectedEvent.location}</span>
          </div>
        {/if}
        {#if selectedEvent.attendees && selectedEvent.attendees.length > 0}
          <div class="flex items-start gap-2 text-xs text-muted-foreground">
            <Icon icon="mdi:account-multiple-outline" class="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span>{selectedEvent.attendees.length} attendee{selectedEvent.attendees.length === 1 ? '' : 's'}</span>
          </div>
        {/if}
        {#if selectedEvent.descriptionHTML}
          <!-- Sanitized by the backend (Core.HTML().Sanitize) before it reaches us -->
          <div class="text-xs leading-relaxed break-words [&_a]:text-primary [&_a]:underline">{@html selectedEvent.descriptionHTML}</div>
        {:else if selectedEvent.description}
          <p class="text-xs leading-relaxed whitespace-pre-wrap break-words">{selectedEvent.description}</p>
        {/if}
      </div>
    </div>
  {:else}
    <!-- Agenda list -->
    <div class="flex-1 overflow-y-auto">
      {#if error}
        <div class="p-3 text-xs text-destructive break-words">{error}</div>
      {:else if loading && instances.length === 0}
        <div class="flex items-center justify-center py-8">
          <Icon icon="mdi:loading" class="w-5 h-5 animate-spin text-muted-foreground" />
        </div>
      {:else if dayGroups.length === 0}
        <div class="flex flex-col items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
          <Icon icon="mdi:calendar-blank-outline" class="w-8 h-8" />
          <p>No upcoming events</p>
        </div>
      {:else}
        {#each dayGroups as group (group.key)}
          <div class="px-3 pt-3 pb-1 text-xs font-medium text-muted-foreground sticky top-0 bg-background">{group.label}</div>
          {#each group.items as inst (inst.id + '-' + inst.instanceStartUnix)}
            <button
              type="button"
              class="w-full text-left px-3 py-2 hover:bg-muted/50 transition-colors flex items-start gap-2"
              onclick={() => openDetail(inst)}
              disabled={detailLoading}
            >
              <span
                class="w-1 self-stretch rounded-full flex-shrink-0"
                style="background-color: {calendarColors[inst.calendarId] || 'var(--primary)'}"
              ></span>
              <span class="flex-1 min-w-0">
                <span class="block text-sm truncate">{inst.summary || '(no title)'}</span>
                <span class="block text-xs text-muted-foreground">{timeLabel(inst)}{inst.location ? ` · ${inst.location}` : ''}</span>
              </span>
            </button>
          {/each}
        {/each}
      {/if}
    </div>
  {/if}
</div>
