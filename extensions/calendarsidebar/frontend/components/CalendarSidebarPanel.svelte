<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Icon from '@iconify/svelte'
  import { isExtensionEnabled, openExtensionSettings } from '$lib/stores/extensionRegistry.svelte'
  import { setActiveExtension } from '$lib/stores/uiState.svelte'
  // @ts-ignore - wailsjs bindings
  import { Calendar_ListSources, Calendar_ListCalendars, Calendar_ListEventsInRange, Calendar_GetEvent, OpenURL } from '$wailsjs/go/app/App.js'
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
  // Selected event detail, shown as a popover floating left of the panel
  // (the pre-extension panel's preview UX). Instance times are kept
  // separately since Calendar_GetEvent returns the master event, whose
  // dtstart is the series start for recurring events.
  let selectedEvent = $state<backend.Event | null>(null)
  let selectedInstance = $state<backend.EventInstance | null>(null)
  let previewRef = $state<HTMLElement | null>(null)

  const calendarEnabled = $derived(isExtensionEnabled('calendar'))

  const dayFmt = new Intl.DateTimeFormat(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
  const timeFmt = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' })

  function dateKey(date: Date): string {
    return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`
  }

  function dayLabel(date: Date): string {
    const today = new Date()
    if (dateKey(date) === dateKey(today)) return 'Today'
    const tomorrow = new Date(today.getTime() + 24 * 60 * 60 * 1000)
    if (dateKey(date) === dateKey(tomorrow)) return 'Tomorrow'
    return dayFmt.format(date)
  }

  function shouldShowDate(index: number): boolean {
    if (index === 0) return true
    const prev = new Date(instances[index - 1].instanceStartUnix * 1000)
    const cur = new Date(instances[index].instanceStartUnix * 1000)
    return dateKey(prev) !== dateKey(cur)
  }

  function timeLabel(startUnix: number, endUnix: number, allDay: boolean): string {
    if (allDay) return 'All day'
    return `${timeFmt.format(new Date(startUnix * 1000))} - ${timeFmt.format(new Date(endUnix * 1000))}`
  }

  // Meeting link detection: Google Meet links are folded into the event
  // description at sync time (see googleConferenceInfo in the calendar
  // extension), so scan location + description for conferencing URLs.
  function meetingLink(ev: { location?: string; description?: string }): string {
    const urls = `${ev.location || ''}\n${ev.description || ''}`.match(/https?:\/\/[^\s<>"']+/g) || []
    const clean = urls.map(url => url.replace(/[),.]+$/, ''))
    return clean.find(url => /meet\.google\.com|teams\.microsoft\.com|zoom\.us/i.test(url)) || ''
  }

  function recurrenceLabel(ev: backend.Event): string {
    const rule = ev.rruleText || ''
    if (!rule) return ''
    if (rule.includes('FREQ=DAILY')) return 'Daily'
    if (rule.includes('FREQ=WEEKLY')) {
      const day = rule.match(/BYDAY=([^;]+)/)?.[1]?.split(',')[0]
      const days: Record<string, string> = {
        MO: 'Monday', TU: 'Tuesday', WE: 'Wednesday', TH: 'Thursday',
        FR: 'Friday', SA: 'Saturday', SU: 'Sunday',
      }
      return day && days[day] ? `Weekly on ${days[day]}` : 'Weekly'
    }
    if (rule.includes('FREQ=MONTHLY')) return 'Monthly'
    if (rule.includes('FREQ=YEARLY')) return 'Yearly'
    return 'Recurring event'
  }

  function attendeeLabel(attendee: backend.Attendee): string {
    return attendee.cn || attendee.email
  }

  function responseCounts(ev: backend.Event) {
    const attendees = ev.attendees || []
    return {
      yes: attendees.filter(a => a.partStat === 'ACCEPTED').length,
      awaiting: attendees.filter(a => !a.partStat || a.partStat === 'NEEDS-ACTION').length,
      tentative: attendees.filter(a => a.partStat === 'TENTATIVE').length,
      declined: attendees.filter(a => a.partStat === 'DECLINED').length,
    }
  }

  function plainDescription(description?: string): string {
    if (!description) return ''
    return description
      .replace(/<br\s*\/?>/gi, '\n')
      .replace(/<\/p>/gi, '\n')
      .replace(/<[^>]+>/g, '')
      .trim()
  }

  async function openLink(url: string) {
    if (!url) return
    try {
      await OpenURL(url)
    } catch (err) {
      console.error('Failed to open link:', err)
    }
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

  async function selectInstance(inst: backend.EventInstance, e: Event) {
    e.stopPropagation()
    if (selectedInstance === inst) {
      closePreview()
      return
    }
    selectedInstance = inst
    try {
      selectedEvent = await Calendar_GetEvent(inst.id)
    } catch {
      // Fall back to the instance data we already have
      selectedEvent = inst
    }
  }

  function closePreview() {
    selectedEvent = null
    selectedInstance = null
  }

  function handleWindowClick(e: MouseEvent) {
    if (!selectedEvent) return
    const target = e.target as HTMLElement | null
    if (!target) return
    if (previewRef?.contains(target)) return
    if (target.closest('[data-calendar-event-card]')) return
    closePreview()
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

<svelte:window onclick={handleWindowClick} />

<aside class="relative h-full w-full bg-background border-l border-border flex flex-col overflow-visible">
  {#if selectedEvent}
    {@const counts = responseCounts(selectedEvent)}
    {@const description = plainDescription(selectedEvent.description)}
    {@const startUnix = selectedInstance?.instanceStartUnix ?? selectedEvent.dtstartUnix}
    {@const endUnix = selectedInstance?.instanceEndUnix ?? selectedEvent.dtendUnix}
    <section
      bind:this={previewRef}
      class="absolute z-50 right-full top-16 mr-2 w-96 max-w-[calc(100vw-2rem)] max-h-[calc(100vh-6rem)] overflow-y-auto scrollbar-thin rounded-md border border-border bg-popover text-popover-foreground shadow-xl"
      aria-label="Calendar event preview"
    >
      <div class="p-4 space-y-4">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 flex items-start gap-3">
            <span
              class="mt-1.5 h-3 w-3 rounded-sm flex-shrink-0"
              style="background-color: {calendarColors[selectedEvent.calendarId] || 'var(--primary)'}"
            ></span>
            <div class="min-w-0">
              <h3 class="text-lg font-semibold leading-6 break-words">{selectedEvent.summary || '(No title)'}</h3>
              <p class="text-sm text-muted-foreground mt-1">
                {dayLabel(new Date(startUnix * 1000))} · {timeLabel(startUnix, endUnix, selectedEvent.isAllDay)}
              </p>
              {#if recurrenceLabel(selectedEvent)}
                <p class="text-sm text-muted-foreground">{recurrenceLabel(selectedEvent)}</p>
              {/if}
            </div>
          </div>
          <button
            class="p-1.5 rounded-md hover:bg-muted transition-colors flex-shrink-0"
            title="Close preview"
            onclick={closePreview}
          >
            <Icon icon="mdi:close" class="w-5 h-5 text-muted-foreground" />
          </button>
        </div>

        {#if meetingLink(selectedEvent)}
          <div class="flex items-center gap-3">
            <Icon icon="mdi:video-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0" />
            <button class="text-sm font-medium text-primary hover:underline" onclick={() => selectedEvent && openLink(meetingLink(selectedEvent))}>
              Join meeting
            </button>
          </div>
        {/if}

        {#if selectedEvent.location}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:map-marker-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <p class="text-sm break-words">{selectedEvent.location}</p>
          </div>
        {/if}

        {#if selectedEvent.attendees && selectedEvent.attendees.length > 0}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:account-group-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <div class="min-w-0 flex-1">
              <p class="text-sm font-medium">
                {selectedEvent.attendees.length} guest{selectedEvent.attendees.length === 1 ? '' : 's'}
              </p>
              <p class="text-xs text-muted-foreground">
                {counts.yes} yes{counts.awaiting ? ` · ${counts.awaiting} awaiting` : ''}{counts.tentative ? ` · ${counts.tentative} tentative` : ''}{counts.declined ? ` · ${counts.declined} declined` : ''}
              </p>
              <div class="mt-3 space-y-2">
                {#each selectedEvent.attendees.slice(0, 8) as attendee (attendee.email)}
                  <div class="flex items-center gap-2 min-w-0">
                    <span class="h-7 w-7 rounded-full bg-primary/20 text-primary flex items-center justify-center text-xs font-medium flex-shrink-0">
                      {attendeeLabel(attendee).slice(0, 1).toUpperCase()}
                    </span>
                    <div class="min-w-0">
                      <p class="text-sm truncate">{attendeeLabel(attendee)}</p>
                      {#if attendee.role === 'CHAIR'}
                        <p class="text-xs text-muted-foreground">Organizer</p>
                      {/if}
                    </div>
                  </div>
                {/each}
                {#if selectedEvent.attendees.length > 8}
                  <p class="text-xs text-muted-foreground">+{selectedEvent.attendees.length - 8} more</p>
                {/if}
              </div>
            </div>
          </div>
        {/if}

        {#if selectedEvent.descriptionHTML}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:text" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <!-- Sanitized by the backend (Core.HTML().Sanitize) before it reaches us -->
            <div class="text-sm leading-relaxed break-words max-h-80 overflow-y-auto scrollbar-thin [&_a]:text-primary [&_a]:underline">{@html selectedEvent.descriptionHTML}</div>
          </div>
        {:else if description}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:text" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <p class="text-sm whitespace-pre-wrap break-words max-h-80 overflow-y-auto scrollbar-thin">{description}</p>
          </div>
        {/if}

        {#if selectedEvent.organizer}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:calendar-account-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <p class="text-sm truncate">{selectedEvent.organizer.cn || selectedEvent.organizer.email}</p>
          </div>
        {/if}
      </div>
    </section>
  {/if}

  <header class="h-10 px-3 border-b border-border flex items-center gap-2 flex-shrink-0">
    <Icon icon="mdi:calendar-month-outline" class="w-4 h-4 text-muted-foreground flex-shrink-0" />
    <h2 class="text-sm font-semibold flex-1 truncate">Upcoming</h2>
    <button
      type="button"
      class="p-1.5 rounded-md hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
      title="Open full calendar"
      onclick={() => setActiveExtension('calendar')}
    >
      <Icon icon="mdi:arrow-expand" class="w-4 h-4" />
    </button>
    <button
      type="button"
      class="p-1.5 rounded-md hover:bg-muted text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
      title="Refresh"
      onclick={() => loadEvents()}
      disabled={loading}
    >
      <Icon icon="mdi:refresh" class="w-4 h-4 {loading ? 'animate-spin' : ''}" />
    </button>
    <button
      type="button"
      class="p-1.5 rounded-md hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
      title="Close"
      onclick={() => onClose?.()}
    >
      <Icon icon="mdi:close" class="w-4 h-4" />
    </button>
  </header>

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
  {:else}
    <div class="flex-1 min-h-0 overflow-y-auto scrollbar-thin">
      {#if error}
        <div class="p-4">
          <div class="rounded-md border border-border bg-muted/40 p-3">
            <p class="text-sm font-medium text-foreground">Calendar unavailable</p>
            <p class="text-sm text-muted-foreground mt-1 break-words">{error}</p>
          </div>
        </div>
      {:else if loading && instances.length === 0}
        <div class="h-full flex items-center justify-center text-sm text-muted-foreground">
          <Icon icon="mdi:loading" class="w-5 h-5 mr-2 animate-spin" />
          Loading events
        </div>
      {:else if instances.length === 0}
        <div class="h-full flex items-center justify-center px-6 text-center text-sm text-muted-foreground">
          No upcoming events.
        </div>
      {:else}
        <div class="p-3 space-y-3">
          {#each instances as inst, index (inst.id + '-' + inst.instanceStartUnix)}
            {@const meetUrl = meetingLink(inst)}
            {@const description = plainDescription(inst.description)}
            {#if shouldShowDate(index)}
              <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide rounded-md bg-muted/50 px-3 py-1.5 mt-4 first:mt-0">
                {dayLabel(new Date(inst.instanceStartUnix * 1000))}
              </h3>
            {/if}
            <div
              data-calendar-event-card
              role="button"
              tabindex="0"
              class="rounded-md border border-border bg-card text-card-foreground p-3 space-y-2 cursor-pointer hover:bg-muted/30 transition-colors {selectedInstance === inst ? 'ring-1 ring-primary border-primary/50' : ''}"
              onclick={(e) => selectInstance(inst, e)}
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectInstance(inst, e) } }}
            >
              <div class="flex items-start gap-2">
                <span
                  class="mt-1 h-2.5 w-2.5 rounded-sm flex-shrink-0"
                  style="background-color: {calendarColors[inst.calendarId] || 'var(--primary)'}"
                ></span>
                <div class="min-w-0 flex-1">
                  <h4 class="text-sm font-medium leading-5 break-words">{inst.summary || '(No title)'}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{timeLabel(inst.instanceStartUnix, inst.instanceEndUnix, inst.isAllDay)}</p>
                </div>
                {#if inst.rruleText}
                  <Icon icon="mdi:repeat" class="w-3.5 h-3.5 text-muted-foreground flex-shrink-0 mt-1" />
                {/if}
              </div>
              {#if inst.location}
                <p class="text-xs text-muted-foreground truncate pl-4">{inst.location}</p>
              {/if}
              {#if description}
                <p class="text-xs text-muted-foreground pl-4 max-h-10 overflow-hidden break-words">{description}</p>
              {/if}
              {#if meetUrl}
                <div class="flex items-center justify-end gap-2 pt-1">
                  <button
                    class="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
                    onclick={(e) => { e.stopPropagation(); openLink(meetUrl) }}
                  >
                    <Icon icon="mdi:video-outline" class="w-4 h-4" />
                    Join
                  </button>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</aside>
