import js from '@eslint/js'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{ts,tsx}', 'tests/**/*.{ts,tsx}'],
    plugins: { 'react-hooks': reactHooks },
    languageOptions: {
      globals: {
        window: 'readonly',
        document: 'readonly',
        fetch: 'readonly',
        RequestInit: 'readonly',
        URL: 'readonly',
        AbortSignal: 'readonly',
        Response: 'readonly',
        TextDecoder: 'readonly',
        KeyboardEvent: 'readonly',
        HTMLButtonElement: 'readonly',
        HTMLInputElement: 'readonly',
        HTMLTextAreaElement: 'readonly',
        HTMLSelectElement: 'readonly',
        globalThis: 'readonly',
      },
    },
    rules: {
      'no-undef': 'off',
      'no-empty': 'off',
      'no-unused-vars': 'off',
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unused-vars': 'error',
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // mutation 失败反馈只有 lib/mutationCache 一个出处：页面内联展示错误的 hook 声明 meta.silentError，
      // 不得在 mutate / useMutation 的 onError、onSettled 里再调 notify.*（会与全局 Toast 叠成两条）
      'no-restricted-syntax': ['error', {
        selector: "Property[key.name=/^on(Error|Settled)$/] CallExpression[callee.object.name='notify']",
        message: 'mutation 失败反馈由 lib/mutationCache 统一弹出；页面内联展示错误请给 hook 声明 meta.silentError，不要在 onError / onSettled 里调用 notify.*。',
      }],
    },
  },
  // 唯一允许在 MutationCache.onError 里弹 Toast 的出处
  {
    files: ['src/lib/mutationCache.ts'],
    rules: { 'no-restricted-syntax': 'off' },
  },
)
