import resolve from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';
import typescript from '@rollup/plugin-typescript';
import terser from '@rollup/plugin-terser';

const production = !process.env.ROLLUP_WATCH;

export default [
  // Browser bundle (UMD)
  {
    input: 'src/index.ts',
    output: [
      {
        file: 'dist/browser/ollystack-rum.js',
        format: 'umd',
        name: 'OllyStack',
        sourcemap: true,
      },
      {
        file: 'dist/browser/ollystack-rum.min.js',
        format: 'umd',
        name: 'OllyStack',
        sourcemap: true,
        plugins: [terser()],
      },
    ],
    plugins: [
      resolve({ browser: true }),
      commonjs(),
      typescript({
        tsconfig: './tsconfig.json',
        declaration: false,
        outDir: 'dist/browser',
      }),
    ],
  },
  // ES Module bundle
  {
    input: 'src/index.ts',
    output: {
      dir: 'dist/esm',
      format: 'es',
      sourcemap: true,
      preserveModules: true,
      preserveModulesRoot: 'src',
    },
    plugins: [
      resolve(),
      commonjs(),
      typescript({
        tsconfig: './tsconfig.json',
        declaration: true,
        declarationDir: 'dist/types',
        outDir: 'dist/esm',
      }),
    ],
  },
  // CommonJS bundle
  {
    input: 'src/index.ts',
    output: {
      dir: 'dist/cjs',
      format: 'cjs',
      sourcemap: true,
      preserveModules: true,
      preserveModulesRoot: 'src',
      exports: 'named',
    },
    plugins: [
      resolve(),
      commonjs(),
      typescript({
        tsconfig: './tsconfig.json',
        declaration: false,
        outDir: 'dist/cjs',
      }),
    ],
  },
];
