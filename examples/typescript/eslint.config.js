import base from './eslint.base.config.js'

// The standard's rules are in eslint.base.config.js, which VibeConform
// manages. Add your own entries after them: a later entry wins, so one
// can add a plugin, add a rule or switch one off. Keep the import:
// vibe audit fails without it, and reports this file as extended once
// it differs from what vibe created (ADR 0024).
export default [...base]
