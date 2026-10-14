import js from '@eslint/js'
import eslintConfigPrettier from 'eslint-config-prettier'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import { existsSync } from 'node:fs'
import path from 'node:path'

// ESLint ignores what the repository's .gitignore ignores, as Prettier
// does, so an agent worktree under .claude/worktrees/ is not linted from
// the main checkout once .gitignore lists it (spec 0045). eslint/config
// exports includeIgnoreFile from ESLint 10.4.0. On an older ESLint, or
// without a .gitignore, nothing is added and the config lints as before.
const { includeIgnoreFile } = await import('eslint/config').catch(() => ({}))
const gitignore = path.join(import.meta.dirname, '.gitignore')
const gitignored = includeIgnoreFile && existsSync(gitignore) ? [includeIgnoreFile(gitignore)] : []

// Every .ts file is type-checked. A narrower glob is faster, but a file
// falling outside it is silently linted by nothing, and a lint run that
// checks less than you think it does still exits 0.
const typedFiles = ['**/*.ts']

// Config files are the exception, and they have to be. Tool configs
// (vitest.config.ts, drizzle.config.ts) normally sit outside every
// tsconfig's include, which under projectService makes them a fatal parse
// error rather than a lint finding. They get the untyped rules instead:
// still linted, just not type-aware.
const configFiles = ['**/*.config.ts', '**/*.config.*.ts']

export default tseslint.config(
  {
    ignores: ['**/dist/**', '**/coverage/**', 'src/contracts/**'],
  },
  ...gitignored,
  {
    ...js.configs.recommended,
    files: ['**/*.{js,mjs}'],
    languageOptions: { globals: globals.node },
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
  eslintConfigPrettier,
)
