// ESLint flat config: type-aware typescript-eslint rules for .ts and .tsx,
// the untyped recommended rules for .js, .mjs and .jsx, and
// eslint-config-prettier last so formatting is Prettier's alone. Add the
// team's own entries at the end of the list: a later entry wins.
//
// Dev dependencies: eslint, @eslint/js, typescript-eslint, globals,
// eslint-config-prettier, eslint-plugin-sonarjs,
// @eslint-community/eslint-plugin-eslint-comments.

import comments from '@eslint-community/eslint-plugin-eslint-comments/configs'
import js from '@eslint/js'
import eslintConfigPrettier from 'eslint-config-prettier'
import sonarjs from 'eslint-plugin-sonarjs'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import { existsSync } from 'node:fs'
import path from 'node:path'

// ESLint ignores what the repository's .gitignore ignores, as Prettier
// does, so an agent worktree under .claude/worktrees/ is not linted from
// the main checkout once .gitignore lists it. eslint/config exports
// includeIgnoreFile from ESLint 10.4.0. On an older ESLint, or without a
// .gitignore, nothing is added and the config lints as before.
const { includeIgnoreFile } = await import('eslint/config').catch(() => ({}))
const gitignore = path.join(import.meta.dirname, '.gitignore')
const gitignored = includeIgnoreFile && existsSync(gitignore) ? [includeIgnoreFile(gitignore)] : []

// Every .ts and .tsx file is type-checked, and every .js, .mjs and .jsx
// file gets the untyped rules: what Prettier formats as source, ESLint
// lints. A narrower glob is faster, but a file falling outside it is
// silently linted by nothing. A typed file must be in tsconfig.json's
// include, which is also where you set jsx.
const typedFiles = ['**/*.{ts,tsx}']

// Config files are the exception, and they have to be. Tool configs
// (vitest.config.ts, drizzle.config.ts) normally sit outside every
// tsconfig's include, which under projectService makes them a fatal parse
// error rather than a lint finding. They get the untyped rules instead:
// still linted, just not type-aware.
const configFiles = ['**/*.config.ts', '**/*.config.*.ts']

export default tseslint.config(
  {
    // The setup skill's directories hold reference configs, not the
    // repository's code: vibe init --skill rewrites them.
    ignores: [
      '**/dist/**',
      '**/coverage/**',
      '.claude/skills/vibeconform-setup/**',
      '.agents/skills/vibeconform-setup/**',
      // Committed generator output: not linted, still type-checked.
      'src/contracts/**',
    ],
  },
  ...gitignored,
  {
    ...js.configs.recommended,
    files: ['**/*.{js,mjs,jsx}'],
    languageOptions: {
      globals: globals.node,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
  },
  ...tseslint.configs.recommendedTypeChecked.map((config) => ({
    ...config,
    files: typedFiles,
    rules: {
      ...config.rules,
      '@typescript-eslint/require-await': 'off',
    },
    languageOptions: {
      ...config.languageOptions,
      globals: globals.node,
      parserOptions: {
        ...config.languageOptions?.parserOptions,
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  })),
  {
    ...tseslint.configs.disableTypeChecked,
    files: configFiles,
    languageOptions: {
      globals: globals.node,
      parserOptions: { projectService: false },
    },
  },
  {
    // Complexity limits catch the contract's planted complexity defect
    // (V-06, a function of cyclomatic complexity 41 and cognitive
    // complexity 40). A function already over a limit gets a specific
    // suppression with a reason,
    // `// eslint-disable-next-line complexity -- baseline: pre-existing complexity`,
    // not a higher limit.
    files: ['**/*.{ts,tsx,js,mjs,jsx}'],
    plugins: { sonarjs },
    rules: {
      complexity: ['error', 15],
      'sonarjs/cognitive-complexity': ['error', 20],
    },
  },
  eslintConfigPrettier,
  // A disable comment names its rules and says why (V-08): a bare
  // eslint-disable would hide every finding in its file. The plugin's
  // no-unused-disable does not report under a flat config, so ESLint's own
  // reportUnusedDisableDirectives reports an unused disable comment.
  comments.recommended,
  {
    rules: {
      '@eslint-community/eslint-comments/no-unlimited-disable': 'error',
      '@eslint-community/eslint-comments/require-description': 'error',
    },
    linterOptions: {
      reportUnusedDisableDirectives: 'error',
    },
  },
  // The team's own entries go here.
)
