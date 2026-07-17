// UI State persistence store
// Handles saving and loading UI state across app sessions

// @ts-ignore - wailsjs bindings
import { GetUIState, SaveUIState } from '../../../wailsjs/go/app/App'
// @ts-ignore - wailsjs bindings
import { appstate } from '../../../wailsjs/go/models'

export interface UIState {
  selectedAccountId: string | null
  selectedFolderId: string | null
  selectedFolderName: string
  selectedFolderType: string | null
  selectedThreadId: string | null
  selectedConversationAccountId: string | null
  selectedConversationFolderId: string | null
  sidebarWidth: number
  listWidth: number
  calendarWidth: number
  calendarOpen: boolean
  windowX: number
  windowY: number
  windowWidth: number
  windowHeight: number
  windowMaximized: boolean
  // Sidebar section expand/collapse states
  expandedAccounts: Record<string, boolean>  // accountId -> isExpanded (default: true)
  unifiedInboxExpanded: boolean              // Unified Inbox section (default: true)
  collapsedFolders: Record<string, boolean>  // folderId -> isCollapsed (default: true/collapsed, false = explicitly expanded)
}

// Pane width constraints
const SIDEBAR_MIN = 180
const SIDEBAR_MAX = 400
const LIST_MIN = 280
const LIST_MAX = 600
const CALENDAR_MIN = 280
const CALENDAR_MAX = 520

// Default state
const defaultState: UIState = {
  selectedAccountId: null,
  selectedFolderId: null,
  selectedFolderName: 'Inbox',
  selectedFolderType: 'inbox',
  selectedThreadId: null,
  selectedConversationAccountId: null,
  selectedConversationFolderId: null,
  sidebarWidth: 240,
  listWidth: 420,
  calendarWidth: 340,
  calendarOpen: false,
  windowX: 0,
  windowY: 0,
  windowWidth: 0,
  windowHeight: 0,
  windowMaximized: false,
  expandedAccounts: {},
  unifiedInboxExpanded: true,
  collapsedFolders: {},
}

// Current state (in-memory cache)
let currentState: UIState = { ...defaultState }

// Reactive signal to notify when UI state has been loaded
// Sidebar can depend on this to re-initialize expanded states
let uiStateLoadedVersion = $state(0)

// Clamp a value within bounds
function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

// Load state from backend on startup
export async function loadUIState(): Promise<UIState> {
  try {
    const state = await GetUIState()
    if (state) {
      // Map from backend model to frontend interface
      // Backend uses camelCase JSON tags that match our interface
      currentState = {
        selectedAccountId: state.selectedAccountId || null,
        selectedFolderId: state.selectedFolderId || null,
        selectedFolderName: state.selectedFolderName || 'Inbox',
        selectedFolderType: state.selectedFolderType || 'inbox',
        selectedThreadId: state.selectedThreadId || null,
        selectedConversationAccountId: state.selectedConversationAccountId || null,
        selectedConversationFolderId: state.selectedConversationFolderId || null,
        // Validate and clamp pane widths
        sidebarWidth: clamp(state.sidebarWidth || 240, SIDEBAR_MIN, SIDEBAR_MAX),
        listWidth: clamp(state.listWidth || 420, LIST_MIN, LIST_MAX),
        calendarWidth: clamp(state.calendarWidth || 340, CALENDAR_MIN, CALENDAR_MAX),
        calendarOpen: state.calendarOpen || false,
        windowX: state.windowX || 0,
        windowY: state.windowY || 0,
        windowWidth: state.windowWidth || 0,
        windowHeight: state.windowHeight || 0,
        windowMaximized: state.windowMaximized || false,
        // Sidebar expand/collapse states
        expandedAccounts: state.expandedAccounts || {},
        unifiedInboxExpanded: state.unifiedInboxExpanded !== false, // default true
        collapsedFolders: state.collapsedFolders || {},
      }
    }
  } catch (err) {
    console.error('Failed to load UI state:', err)
  }
  // Increment version to trigger reactive updates in components waiting for state
  uiStateLoadedVersion++
  return currentState
}

// Get the reactive version number (components can depend on this to re-run effects when state loads)
export function getUIStateVersion(): number {
  return uiStateLoadedVersion
}

// Debounced save
let saveTimer: ReturnType<typeof setTimeout> | null = null

async function persistCurrentState(): Promise<void> {
  try {
    // Convert to backend model format
    const backendState: appstate.UIState = {
      selectedAccountId: currentState.selectedAccountId || '',
      selectedFolderId: currentState.selectedFolderId || '',
      selectedFolderName: currentState.selectedFolderName,
      selectedFolderType: currentState.selectedFolderType || '',
      selectedThreadId: currentState.selectedThreadId || '',
      selectedConversationAccountId: currentState.selectedConversationAccountId || '',
      selectedConversationFolderId: currentState.selectedConversationFolderId || '',
      sidebarWidth: currentState.sidebarWidth,
      listWidth: currentState.listWidth,
      calendarWidth: currentState.calendarWidth,
      calendarOpen: currentState.calendarOpen,
      windowX: currentState.windowX,
      windowY: currentState.windowY,
      windowWidth: currentState.windowWidth,
      windowHeight: currentState.windowHeight,
      windowMaximized: currentState.windowMaximized,
      expandedAccounts: currentState.expandedAccounts,
      unifiedInboxExpanded: currentState.unifiedInboxExpanded,
      collapsedFolders: currentState.collapsedFolders,
    }
    await SaveUIState(backendState)
  } catch (err) {
    console.error('Failed to save UI state:', err)
  }
}

export function saveUIState(updates: Partial<UIState>, immediate = false): void {
  // Merge updates into current state
  currentState = { ...currentState, ...updates }

  // Clamp pane widths if updated
  if (updates.sidebarWidth !== undefined) {
    currentState.sidebarWidth = clamp(updates.sidebarWidth, SIDEBAR_MIN, SIDEBAR_MAX)
  }
  if (updates.listWidth !== undefined) {
    currentState.listWidth = clamp(updates.listWidth, LIST_MIN, LIST_MAX)
  }
  if (updates.calendarWidth !== undefined) {
    currentState.calendarWidth = clamp(updates.calendarWidth, CALENDAR_MIN, CALENDAR_MAX)
  }

  if (immediate) {
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = null
    void persistCurrentState()
    return
  }

  // Debounce: save at most once per second
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    saveTimer = null
    void persistCurrentState()
  }, 1000)
}

export function flushUIState(): Promise<void> {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = null
  return persistCurrentState()
}

// Helper to check if an account is expanded (defaults to true if not set)
export function isAccountExpanded(accountId: string): boolean {
  return currentState.expandedAccounts[accountId] !== false
}

// Helper to set account expanded state
export function setAccountExpanded(accountId: string, expanded: boolean): void {
  const newExpandedAccounts = { ...currentState.expandedAccounts, [accountId]: expanded }
  saveUIState({ expandedAccounts: newExpandedAccounts })
}

// Helper to check if unified inbox is expanded
export function isUnifiedInboxExpanded(): boolean {
  return currentState.unifiedInboxExpanded !== false
}

// Helper to set unified inbox expanded state
export function setUnifiedInboxExpanded(expanded: boolean): void {
  saveUIState({ unifiedInboxExpanded: expanded })
}

// Helper to check if a folder is collapsed (defaults to true/collapsed if not set)
export function isFolderCollapsed(folderId: string): boolean {
  return currentState.collapsedFolders[folderId] !== false
}

// Helper to set folder collapsed state
export function setFolderCollapsed(folderId: string, collapsed: boolean): void {
  const newCollapsedFolders = { ...currentState.collapsedFolders, [folderId]: collapsed }
  saveUIState({ collapsedFolders: newCollapsedFolders })
}

// Get current state (synchronous)
export function getUIState(): UIState {
  return currentState
}

// Get pane width constraints (for UI components)
export const paneConstraints = {
  sidebar: { min: SIDEBAR_MIN, max: SIDEBAR_MAX },
  list: { min: LIST_MIN, max: LIST_MAX },
  calendar: { min: CALENDAR_MIN, max: CALENDAR_MAX },
}
