<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useTemplateRef } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import { usePreferences } from '../../stores/preferences'
import ShortcutSettings from './ShortcutSettings.vue'

/** The preferences are applied live as the user toggles them, mirroring the
 *  IDEA settings dialog where controls commit their value immediately.
 *  Sections are split into tabs so the dialog opens on one page instead of
 *  every option at once. */
const emit = defineEmits<{
  close: []
  'forget-session': []
  'toggle-fullscreen': []
}>()
defineProps<{ sessionNotice?: string }>()

type SettingsTab = 'appearance' | 'editor' | 'versionControl' | 'git' | 'startup' | 'shortcuts'
const tabs: { id: SettingsTab; label: string }[] = [
  { id: 'appearance', label: 'Appearance' },
  { id: 'editor', label: 'Editor' },
  { id: 'versionControl', label: 'Version Control' },
  { id: 'git', label: 'Git' },
  { id: 'startup', label: 'Startup & Window' },
  { id: 'shortcuts', label: 'Keymap' },
]
const tab = ref<SettingsTab>('appearance')
const tabList = useTemplateRef<HTMLElement>('tab-list')

/** Left/Right/Home/End move between tabs, as in a native tablist. */
function stepTab(event: KeyboardEvent, offset: number | 'first' | 'last') {
  const index = tabs.findIndex(item => item.id === tab.value)
  const next = offset === 'first' ? 0
    : offset === 'last' ? tabs.length - 1
      : (index + offset + tabs.length) % tabs.length
  tab.value = tabs[next].id
  event.preventDefault()
  const button = tabList.value?.querySelectorAll<HTMLButtonElement>('[role=tab]')[next]
  button?.focus()
}

const preferences = usePreferences()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const gitVersion = ref('')
const gitError = ref('')
const gitBusy = ref(false)

/** The Git binary is validated by the backend before it replaces the current
 *  one, so an invalid path cannot break an open repository. */
async function applyGitPath() {
  if (gitBusy.value) return
  gitBusy.value = true
  gitError.value = ''
  try {
    gitVersion.value = await api.ConfigureGit(preferences.gitPath)
  } catch (cause) {
    gitVersion.value = ''
    gitError.value = String(cause)
  } finally {
    gitBusy.value = false
  }
}

function close() {
  emit('close')
}

onMounted(() => {
  dialog.value?.showModal()
  dialog.value?.querySelector<HTMLElement>('[data-initial-focus]')?.focus()
})

onBeforeUnmount(() => dialog.value?.close())
</script>

<template>
  <dialog ref="dialog" class="settings-dialog" aria-label="Settings" @cancel.prevent="close" @keydown.stop>
    <header>
      <h2>Settings</h2>
      <button type="button" aria-label="Close settings" tabindex="-1" @click="close">×</button>
    </header>
    <div class="settings-layout">
      <nav ref="tab-list" class="settings-tabs" role="tablist" aria-orientation="vertical" aria-label="Settings sections"
           @keydown.up.prevent="stepTab($event, -1)" @keydown.down.prevent="stepTab($event, 1)"
           @keydown.left.prevent="stepTab($event, -1)" @keydown.right.prevent="stepTab($event, 1)"
           @keydown.home.prevent="stepTab($event, 'first')" @keydown.end.prevent="stepTab($event, 'last')">
        <button v-for="item in tabs" :key="item.id" type="button" role="tab" :id="'settings-tab-' + item.id"
                :aria-selected="tab === item.id" :aria-controls="'settings-panel-' + item.id"
                :class="{ active: tab === item.id }" :tabindex="tab === item.id ? 0 : -1"
                @click="tab = item.id">{{ item.label }}</button>
      </nav>
      <div class="dialog-body settings-body">
        <section v-if="tab === 'appearance'" id="settings-panel-appearance" class="settings-group"
                 role="tabpanel" aria-labelledby="settings-tab-appearance">
          <h3>Appearance</h3>
          <label><input v-model="preferences.dark" type="checkbox" /> Dark theme</label>
          <label><input type="checkbox" :checked="preferences.density === 'compact'" @change="preferences.toggleDensity()" /> Compact rows</label>
          <label><input v-model="preferences.fileTree" type="checkbox" /> Show Changed Files as a tree</label>
        </section>
        <section v-if="tab === 'editor'" id="settings-panel-editor" class="settings-group"
                 role="tabpanel" aria-labelledby="settings-tab-editor">
          <h3>Editor</h3>
          <label><input v-model="preferences.editorWordWrap" type="checkbox" /> Soft wrap long lines</label>
          <label class="settings-field">Font size
            <input v-model.number="preferences.editorFontSize" type="number" min="10" max="24" step="1"
                   aria-label="Editor font size in pixels" />
          </label>
          <label class="settings-field">Tab size
            <select v-model.number="preferences.editorTabSize" aria-label="Editor tab size">
              <option :value="2">2 spaces</option>
              <option :value="4">4 spaces</option>
              <option :value="8">8 spaces</option>
            </select>
          </label>
          <p class="notice">Changes apply to open editors without reloading the file.</p>
        </section>
        <section v-if="tab === 'versionControl'" id="settings-panel-versionControl" class="settings-group"
                 role="tabpanel" aria-labelledby="settings-tab-versionControl">
          <h3>Version Control</h3>
          <label class="settings-field">Issue pattern
            <input v-model="preferences.issuePattern" type="text" spellcheck="false"
                   placeholder="e.g. [A-Z]+-\d+ — leave empty to disable"
                   aria-label="Issue ID pattern in commit messages" />
          </label>
          <label class="settings-field">Issue URL
            <input v-model="preferences.issueURL" type="text" spellcheck="false"
                   placeholder="e.g. https://youtrack.example.com/issue/$1"
                   aria-label="Issue URL template; $0 match, $1 first group" />
          </label>
          <p class="notice">Commit messages link issue IDs matching the pattern to the URL ($0 = whole match, $1–$9 = groups).</p>
          <label class="settings-field">Protected branches
            <textarea v-model="preferences.protectedBranches" rows="3" spellcheck="false"
                      placeholder="main, master, release/*"
                      aria-label="Protected branch patterns" />
          </label>
          <p class="notice">Exact names or wildcards (release/*), one per line or comma separated. Force pushes to them are refused.</p>
        </section>
        <section v-if="tab === 'git'" id="settings-panel-git" class="settings-group"
                 role="tabpanel" aria-labelledby="settings-tab-git">
          <h3>Git</h3>
          <label class="settings-field">Git executable
            <input v-model="preferences.gitPath" type="text" spellcheck="false"
                   placeholder="Leave empty to use Git from PATH"
                   aria-label="Git executable path" />
          </label>
          <button type="button" :disabled="gitBusy" @click="applyGitPath">{{ gitBusy ? 'Checking…' : 'Apply executable' }}</button>
          <p v-if="gitVersion" class="notice" role="status">{{ gitVersion }}</p>
          <p v-if="gitError" class="notice error" role="alert">{{ gitError }}</p>
          <p class="notice">Every Git command runs through this executable. Applying an invalid path is refused and leaves the current one in place.</p>
        </section>
        <section v-if="tab === 'startup'" id="settings-panel-startup" class="settings-group"
                 role="tabpanel" aria-labelledby="settings-tab-startup">
          <h3>Startup</h3>
          <label><input v-model="preferences.resumeOnStart" type="checkbox" /> Reopen last repository on startup</label>
          <h3>Window</h3>
          <button type="button" @click="emit('toggle-fullscreen')">Toggle fullscreen</button>
          <button type="button" @click="preferences.resetLayout()">Reset panel sizes</button>
          <h3>Session</h3>
          <button type="button" @click="emit('forget-session')">Forget saved session</button>
          <p v-if="sessionNotice" class="notice" role="status">{{ sessionNotice }}</p>
        </section>
        <section v-if="tab === 'shortcuts'" id="settings-panel-shortcuts" class="settings-group"
                 role="tabpanel" aria-labelledby="settings-tab-shortcuts">
          <ShortcutSettings />
        </section>
      </div>
    </div>
    <footer class="dialog-actions">
      <button type="button" class="primary" data-initial-focus @click="close">Close</button>
    </footer>
  </dialog>
</template>
