export default [
  { ignores: ['dist/**', 'node_modules/**', 'src/**/*.{ts,tsx}'] },
  { files: ['**/*.js'], rules: { 'no-console': 'warn' } }
];
