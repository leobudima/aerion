<script lang="ts">
  import Icon from '@iconify/svelte'
  import { format, isToday, isTomorrow, isSameDay } from 'date-fns'
  import { Button } from '$lib/components/ui/button'
  import { accountStore } from '$lib/stores/accounts.svelte'
  import { oauthStore } from '$lib/stores/oauth.svelte'
  import { addToast } from '$lib/stores/toast'
  // @ts-ignore - generated bindings are root-owned in this checkout
  import { OpenURL } from '../../../../wailsjs/go/app/App'

  interface CalendarEvent {
    id: string
    summary: string
    description?: string
    location?: string
    start: string
    end: string
    allDay: boolean
    htmlLink: string
    hangoutLink?: string
    organizer?: CalendarPerson
    creator?: CalendarPerson
    attendees?: CalendarAttendee[]
    recurringEvent?: boolean
    recurrence?: string[]
    reminderText?: string
    accountId: string
    accountName: string
    accountEmail: string
  }

  interface CalendarPerson {
    email?: string
    name?: string
    self?: boolean
  }

  interface CalendarAttendee {
    email: string
    name?: string
    responseStatus?: string
    optional?: boolean
    organizer?: boolean
    self?: boolean
  }

  interface Props {
    accountId?: string | null
    onClose?: () => void
  }

  let {
    accountId = null,
    onClose,
  }: Props = $props()

  let events = $state<CalendarEvent[]>([])
  let loading = $state(false)
  let error = $state<string | null>(null)
  let needsReauth = $state(false)
  let resolvedAccountId = $state<string | null>(null)
  let lastRequestedAccountId = $state<string | null>(null)
  let selectedEvent = $state<CalendarEvent | null>(null)
  let previewRef = $state<HTMLElement | null>(null)

  const googleAccounts = $derived(accountStore.accounts
    .map(item => item.account)
    .filter(account => account.authType === 'oauth2' && account.imapHost.toLowerCase().includes('gmail')))

  const selectedGoogleAccountId = $derived((() => {
    if (accountId && accountId !== 'unified' && googleAccounts.some(account => account.id === accountId)) {
      return accountId
    }
    return googleAccounts[0]?.id ?? null
  })())

  $effect(() => {
    const id = selectedGoogleAccountId
    if (!id) {
      events = []
      selectedEvent = null
      resolvedAccountId = null
      lastRequestedAccountId = null
      error = googleAccounts.length === 0 ? 'No Google OAuth account found.' : null
      needsReauth = false
      return
    }
    if (id !== lastRequestedAccountId) {
      lastRequestedAccountId = id
      loadEvents(selectedGoogleAccountId)
    }
  })

  async function listUpcomingCalendarEvents(id: string, maxResults: number): Promise<CalendarEvent[]> {
    return (window as any).go.app.App.ListUpcomingCalendarEvents(id, maxResults)
  }

  async function loadEvents(id = selectedGoogleAccountId) {
    if (!id || loading) return

    loading = true
    error = null
    needsReauth = false

    try {
      const result = await listUpcomingCalendarEvents(id, 20)
      events = result || []
      if (selectedEvent && !events.some(event => event.id === selectedEvent?.id)) {
        selectedEvent = null
      }
      resolvedAccountId = result?.[0]?.accountId || id
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      error = message
      needsReauth = message.toLowerCase().includes('calendar permission is missing')
      events = []
      selectedEvent = null
      resolvedAccountId = id
    } finally {
      loading = false
    }
  }

  async function handleReauthorize() {
    if (!resolvedAccountId && !selectedGoogleAccountId) return

    try {
      await oauthStore.reauthorize((resolvedAccountId || selectedGoogleAccountId)!)
      addToast({ type: 'success', message: 'Google Calendar access granted.' })
      await loadEvents((resolvedAccountId || selectedGoogleAccountId)!)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      addToast({ type: 'error', message: message || 'Failed to re-authorize Google account.' })
    }
  }

  async function openEvent(event: CalendarEvent) {
    const url = event.htmlLink || event.hangoutLink
    if (!url) return
    try {
      await OpenURL(url)
    } catch (err) {
      console.error('Failed to open calendar event:', err)
    }
  }

  async function joinEvent(event: CalendarEvent) {
    const url = meetingLink(event)
    if (!url) return
    try {
      await OpenURL(url)
    } catch (err) {
      console.error('Failed to open meeting link:', err)
    }
  }

  function handleEventClick(event: CalendarEvent, e: MouseEvent) {
    e.stopPropagation()
    selectedEvent = event
  }

  function handleEventKeydown(event: CalendarEvent, e: KeyboardEvent) {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    selectedEvent = event
  }

  function handleWindowClick(e: MouseEvent) {
    if (!selectedEvent) return
    const target = e.target as HTMLElement | null
    if (!target) return
    if (previewRef?.contains(target)) return
    if (target.closest('[data-calendar-event-card]')) return
    selectedEvent = null
  }

  function eventDateLabel(event: CalendarEvent): string {
    const start = new Date(event.start)
    if (isToday(start)) return 'Today'
    if (isTomorrow(start)) return 'Tomorrow'
    return format(start, 'EEE, MMM d')
  }

  function eventTimeLabel(event: CalendarEvent): string {
    if (event.allDay) return 'All day'

    const start = new Date(event.start)
    const end = new Date(event.end)
    if (Number.isNaN(start.getTime())) return ''
    if (Number.isNaN(end.getTime())) return format(start, 'p')

    const startLabel = format(start, 'p')
    const endLabel = format(end, 'p')
    return `${startLabel} - ${endLabel}`
  }

  function shouldShowDate(index: number): boolean {
    if (index === 0) return true
    return !isSameDay(new Date(events[index - 1].start), new Date(events[index].start))
  }

  function meetingLink(event: CalendarEvent): string {
    if (event.hangoutLink) return event.hangoutLink
    const urls = `${event.location || ''}\n${event.description || ''}`.match(/https?:\/\/[^\s<>"']+/g) || []
    const cleanUrls = urls.map(url => url.replace(/[),.]+$/, ''))
    return cleanUrls.find(url => /meet\.google\.com|teams\.microsoft\.com|zoom\.us/i.test(url)) || cleanUrls[0] || ''
  }

  function attendeeLabel(attendee: CalendarAttendee): string {
    return attendee.name || attendee.email
  }

  function personLabel(person?: CalendarPerson): string {
    if (!person) return ''
    return person.name || person.email || ''
  }

  function responseCounts(event: CalendarEvent) {
    const attendees = event.attendees || []
    return {
      yes: attendees.filter(attendee => attendee.responseStatus === 'accepted').length,
      awaiting: attendees.filter(attendee => attendee.responseStatus === 'needsAction').length,
      tentative: attendees.filter(attendee => attendee.responseStatus === 'tentative').length,
      declined: attendees.filter(attendee => attendee.responseStatus === 'declined').length,
    }
  }

  function recurrenceLabel(event: CalendarEvent): string {
    if (!event.recurringEvent) return ''
    const rule = event.recurrence?.find(item => item.startsWith('RRULE:')) || ''
    if (rule.includes('FREQ=DAILY')) return 'Daily'
    if (rule.includes('FREQ=WEEKLY')) {
      const day = rule.match(/BYDAY=([^;]+)/)?.[1]?.split(',')[0]
      const days: Record<string, string> = {
        MO: 'Monday',
        TU: 'Tuesday',
        WE: 'Wednesday',
        TH: 'Thursday',
        FR: 'Friday',
        SA: 'Saturday',
        SU: 'Sunday',
      }
      return day && days[day] ? `Weekly on ${days[day]}` : 'Weekly'
    }
    if (rule.includes('FREQ=MONTHLY')) return 'Monthly'
    if (rule.includes('FREQ=YEARLY')) return 'Yearly'
    return 'Recurring event'
  }

  function plainDescription(description?: string): string {
    if (!description) return ''
    return description
      .replace(/<br\s*\/?>/gi, '\n')
      .replace(/<\/p>/gi, '\n')
      .replace(/<[^>]+>/g, '')
      .trim()
  }
</script>

<svelte:window onclick={handleWindowClick} />

<aside class="relative h-full w-full bg-background border-l border-border flex flex-col overflow-visible">
  {#if selectedEvent}
    {@const counts = responseCounts(selectedEvent)}
    {@const description = plainDescription(selectedEvent.description)}
    <section
      bind:this={previewRef}
      class="absolute z-50 right-full top-16 mr-2 w-96 max-w-[calc(100vw-2rem)] max-h-[calc(100vh-6rem)] overflow-y-auto scrollbar-thin rounded-md border border-border bg-popover text-popover-foreground shadow-xl"
      aria-label="Calendar event preview"
    >
      <div class="p-4 space-y-4">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 flex items-start gap-3">
            <span class="mt-1.5 h-3 w-3 rounded-sm bg-primary flex-shrink-0"></span>
            <div class="min-w-0">
              <h3 class="text-lg font-semibold leading-6 break-words">{selectedEvent.summary || '(No title)'}</h3>
              <p class="text-sm text-muted-foreground mt-1">
                {eventDateLabel(selectedEvent)} · {eventTimeLabel(selectedEvent)}
              </p>
              {#if recurrenceLabel(selectedEvent)}
                <p class="text-sm text-muted-foreground">{recurrenceLabel(selectedEvent)}</p>
              {/if}
            </div>
          </div>
          <button
            class="p-1.5 rounded-md hover:bg-muted transition-colors flex-shrink-0"
            title="Close preview"
            onclick={() => selectedEvent = null}
          >
            <Icon icon="mdi:close" class="w-5 h-5 text-muted-foreground" />
          </button>
        </div>

        {#if meetingLink(selectedEvent)}
          <div class="flex items-center gap-3">
            <Icon icon="mdi:video-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0" />
            <button class="text-sm font-medium text-primary hover:underline" onclick={() => selectedEvent && joinEvent(selectedEvent)}>
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
                {selectedEvent.attendees.length} guests
              </p>
              <p class="text-xs text-muted-foreground">
                {counts.yes} yes{counts.awaiting ? ` · ${counts.awaiting} awaiting` : ''}{counts.tentative ? ` · ${counts.tentative} tentative` : ''}{counts.declined ? ` · ${counts.declined} declined` : ''}
              </p>
              <div class="mt-3 space-y-2">
                {#each selectedEvent.attendees.slice(0, 8) as attendee}
                  <div class="flex items-center gap-2 min-w-0">
                    <span class="h-7 w-7 rounded-full bg-primary/20 text-primary flex items-center justify-center text-xs font-medium flex-shrink-0">
                      {attendeeLabel(attendee).slice(0, 1).toUpperCase()}
                    </span>
                    <div class="min-w-0">
                      <p class="text-sm truncate">{attendeeLabel(attendee)}</p>
                      {#if attendee.organizer}
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

        {#if description}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:text" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <p class="text-sm whitespace-pre-wrap break-words max-h-80 overflow-y-auto scrollbar-thin">{description}</p>
          </div>
        {/if}

        {#if selectedEvent.reminderText}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:bell-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <p class="text-sm">{selectedEvent.reminderText}</p>
          </div>
        {/if}

        {#if personLabel(selectedEvent.creator)}
          <div class="flex items-start gap-3">
            <Icon icon="mdi:calendar-account-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0 mt-0.5" />
            <div class="min-w-0">
              <p class="text-sm truncate">{personLabel(selectedEvent.organizer) || personLabel(selectedEvent.creator)}</p>
              {#if personLabel(selectedEvent.creator) && personLabel(selectedEvent.creator) !== personLabel(selectedEvent.organizer)}
                <p class="text-xs text-muted-foreground truncate">Created by: {personLabel(selectedEvent.creator)}</p>
              {/if}
            </div>
          </div>
        {/if}
      </div>
    </section>
  {/if}

  <header class="h-14 px-3 border-b border-border flex items-center justify-between gap-2 flex-shrink-0">
    <div class="min-w-0">
      <div class="flex items-center gap-2">
        <Icon icon="mdi:calendar-month-outline" class="w-5 h-5 text-muted-foreground flex-shrink-0" />
        <h2 class="text-sm font-semibold text-foreground truncate">Calendar</h2>
      </div>
      {#if googleAccounts.length > 0}
        <p class="text-xs text-muted-foreground truncate">
          {googleAccounts.find(account => account.id === selectedGoogleAccountId)?.email}
        </p>
      {/if}
    </div>
    <div class="flex items-center gap-1 flex-shrink-0">
      <button
        class="p-2 rounded-md hover:bg-muted transition-colors disabled:opacity-50"
        title="Refresh calendar"
        disabled={loading || !selectedGoogleAccountId}
        onclick={() => loadEvents()}
      >
        <Icon icon="mdi:refresh" class="w-5 h-5 text-muted-foreground {loading ? 'animate-spin' : ''}" />
      </button>
      <button
        class="p-2 rounded-md hover:bg-muted transition-colors"
        title="Close calendar"
        onclick={onClose}
      >
        <Icon icon="mdi:close" class="w-5 h-5 text-muted-foreground" />
      </button>
    </div>
  </header>

  <div class="flex-1 min-h-0 overflow-y-auto scrollbar-thin">
    {#if loading && events.length === 0}
      <div class="h-full flex items-center justify-center text-sm text-muted-foreground">
        <Icon icon="mdi:loading" class="w-5 h-5 mr-2 animate-spin" />
        Loading events
      </div>
    {:else if error}
      <div class="p-4 space-y-3">
        <div class="rounded-md border border-border bg-muted/40 p-3">
          <p class="text-sm font-medium text-foreground">Calendar unavailable</p>
          <p class="text-sm text-muted-foreground mt-1">{error}</p>
        </div>
        {#if needsReauth}
          <Button variant="outline" class="w-full" onclick={handleReauthorize}>
            <Icon icon="mdi:login" class="w-4 h-4 mr-2" />
            Sign in again
          </Button>
        {/if}
      </div>
    {:else if events.length === 0}
      <div class="h-full flex items-center justify-center px-6 text-center text-sm text-muted-foreground">
        No upcoming events.
      </div>
    {:else}
      <div class="p-3 space-y-3">
        {#each events as event, index (event.id)}
          {#if shouldShowDate(index)}
            <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide rounded-md bg-muted/50 px-3 py-1.5 mt-4 first:mt-0">
              {eventDateLabel(event)}
            </h3>
          {/if}
          <div
            data-calendar-event-card
            role="button"
            tabindex="0"
            class="rounded-md border border-border bg-card text-card-foreground p-3 space-y-2 cursor-pointer hover:bg-muted/30 transition-colors {selectedEvent?.id === event.id ? 'ring-1 ring-primary border-primary/50' : ''}"
            onclick={(e) => handleEventClick(event, e)}
            onkeydown={(e) => handleEventKeydown(event, e)}
          >
            <div class="flex items-start gap-2">
              <span class="mt-1 h-2.5 w-2.5 rounded-sm bg-primary flex-shrink-0"></span>
              <div class="min-w-0 flex-1">
                <h4 class="text-sm font-medium leading-5 break-words">{event.summary || '(No title)'}</h4>
                <p class="text-xs text-muted-foreground mt-0.5">{eventTimeLabel(event)}</p>
              </div>
            </div>
            {#if event.location}
              <p class="text-xs text-muted-foreground truncate pl-4">{event.location}</p>
            {/if}
            {#if event.description}
              <p class="text-xs text-muted-foreground pl-4 max-h-10 overflow-hidden break-words">{plainDescription(event.description)}</p>
            {/if}
            <div class="flex items-center justify-end gap-2 pt-1">
              {#if event.htmlLink}
                <button
                  class="inline-flex items-center gap-1 rounded-md px-2.5 py-1.5 text-xs font-medium border border-border bg-background hover:bg-accent hover:text-accent-foreground transition-colors"
                  onclick={(e) => { e.stopPropagation(); openEvent(event) }}
                >
                  <Icon icon="mdi:open-in-new" class="w-3.5 h-3.5" />
                  Open
                </button>
              {/if}
              {#if meetingLink(event)}
                <button
                  class="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
                  onclick={(e) => { e.stopPropagation(); joinEvent(event) }}
                >
                  <Icon icon="mdi:video-outline" class="w-4 h-4" />
                  Join
                </button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</aside>
