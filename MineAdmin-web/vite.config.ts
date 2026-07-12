import path from 'node:path'
import process from 'node:process'
import dayjs from 'dayjs'
import { defineConfig, loadEnv } from 'vite'
import pkg from './package.json'
import createVitePlugins from './vite'
import { exclude, include } from './vite/optimize'

const pathSeparator = /[\\/]/

function includesNodeModule(id: string, moduleName: string): boolean {
  return id
    .split(pathSeparator)
    .join('/')
    .includes(`/node_modules/${moduleName}/`)
}

function manualChunks(id: string): string | undefined {
  const normalizedId = id.split(pathSeparator).join('/')
  if (!normalizedId.includes('/node_modules/')) {
    return undefined
  }

  if (includesNodeModule(normalizedId, 'zrender')) {
    return 'vendor-zrender'
  }
  if (includesNodeModule(normalizedId, 'echarts/lib/chart')) {
    return 'vendor-echarts-charts'
  }
  if (includesNodeModule(normalizedId, 'echarts/lib/component')) {
    return 'vendor-echarts-components'
  }
  if (includesNodeModule(normalizedId, 'echarts/lib/coord')) {
    return 'vendor-echarts-coord'
  }
  if (includesNodeModule(normalizedId, 'echarts')) {
    return 'vendor-echarts-core'
  }
  if (
    includesNodeModule(normalizedId, 'vue')
    || includesNodeModule(normalizedId, 'vue-router')
    || includesNodeModule(normalizedId, 'pinia')
    || includesNodeModule(normalizedId, 'vue-i18n')
  ) {
    return 'vendor-vue'
  }
  if (includesNodeModule(normalizedId, 'element-plus')) {
    return 'vendor-element-plus'
  }
  if (includesNodeModule(normalizedId, '@mineadmin')) {
    return 'vendor-mineadmin'
  }
  if (includesNodeModule(normalizedId, '@iconify')) {
    return 'vendor-iconify'
  }
  if (
    includesNodeModule(normalizedId, '@floating-ui')
    || includesNodeModule(normalizedId, 'floating-vue')
    || includesNodeModule(normalizedId, 'radix-vue')
    || includesNodeModule(normalizedId, 'reka-ui')
    || includesNodeModule(normalizedId, 'vaul-vue')
    || includesNodeModule(normalizedId, 'overlayscrollbars')
    || includesNodeModule(normalizedId, 'overlayscrollbars-vue')
    || includesNodeModule(normalizedId, '@imengyu/vue3-context-menu')
  ) {
    return 'vendor-ui'
  }
  if (
    includesNodeModule(normalizedId, 'axios')
    || includesNodeModule(normalizedId, 'dayjs')
    || includesNodeModule(normalizedId, 'lodash-es')
    || includesNodeModule(normalizedId, 'nprogress')
    || includesNodeModule(normalizedId, 'path-browserify')
    || includesNodeModule(normalizedId, 'qs')
    || includesNodeModule(normalizedId, 'radash')
    || includesNodeModule(normalizedId, 'sortablejs')
    || includesNodeModule(normalizedId, 'web-storage-cache')
  ) {
    return 'vendor-utils'
  }

  return 'vendor'
}

function ignoreKnownThirdPartyWarnings(
  warning: any,
  defaultHandler: (warning: any) => void,
): void {
  const source = `${warning.id ?? ''} ${warning.loc?.file ?? ''} ${
    warning.message ?? ''
  }`
    .split(pathSeparator)
    .join('/')
  if (
    warning.code === 'INVALID_ANNOTATION'
    && /node_modules\/(?:element-plus|reka-ui)\/node_modules\/@vueuse\/core\/dist\/index\.js/.test(
      source,
    )
  ) {
    return
  }
  defaultHandler(warning)
}

// https://cn.vite.dev/config/
export default async ({ mode, command }) => {
  const env = loadEnv(mode, process.cwd())
  function isProduction(): boolean {
    return mode === 'production'
  }

  // 全局 scss 资源
  // const scssFiles: string[] = []
  // fs.readdirSync('src/assets/styles/resources').forEach((dirname) => {
  //   if (fs.statSync(`src/assets/styles/resources/${dirname}`).isFile()) {
  //     scssFiles.push(`@use "src/assets/styles/resources/${dirname}" as *;`)
  //   }
  // })

  const proxyPrefix = env.VITE_PROXY_PREFIX
  return defineConfig({
    base: env.VITE_APP_ROOT_BASE,
    // 开发服务器选项 https://cn.vite.dev/config/#server-options
    server: {
      host: '0.0.0.0',
      open: true,
      port: Number(env.VITE_APP_PORT ?? process.env.port),
      warmup: {
        clientFiles: [
          './src/layouts/**/*.{ts,tsx,vue}',
          './src/modules/**/views/**/*.{ts,tsx,vue}',
        ],
      },
      proxy: {
        [proxyPrefix]: {
          target: env.VITE_APP_API_BASEURL,
          changeOrigin: command === 'serve' && env.VITE_OPEN_PROXY === 'true',
          rewrite: path => path.replace(new RegExp(`^${proxyPrefix}`), ''),
          configure: (proxy) => {
            proxy.on('proxyReq', (proxyReq, req) => {
              const host = req.headers.host
              if (host) {
                proxyReq.setHeader('X-Forwarded-Host', host)
              }
            })
          },
        },
      },
    },
    oxc: {
      dropConsole: isProduction(),
      dropDebugger: isProduction(),
    },
    // 构建选项 https://cn.vite.dev/config/#server-fsserve-root
    build: {
      outDir: isProduction ? 'dist' : `dist-${mode}`,
      sourcemap: env.VITE_BUILD_SOURCEMAP === 'true',
      minify: 'esbuild',
      cssMinify: 'esbuild',
      chunkSizeWarningLimit: 1000,
      rolldownOptions: {
        checks: {
          pluginTimings: false,
        },
        onwarn: ignoreKnownThirdPartyWarnings,
        output: {
          codeSplitting: true,
          chunkFileNames: 'static/js/[name]-[hash].js',
          entryFileNames: 'static/js/[name]-[hash].js',
          assetFileNames: 'static/[ext]/[name]-[hash].[ext]',
          manualChunks,
        },
      },
    },
    define: {
      __MINE_SYSTEM_INFO__: JSON.stringify({
        pkg: {
          version: pkg.version,
          dependencies: pkg.dependencies,
          devDependencies: pkg.devDependencies,
        },
        lastBuildTime: dayjs().format('YYYY-MM-DD HH:mm:ss'),
      }),
    },
    plugins: createVitePlugins(env, command === 'build'),
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src'),
        '#': path.resolve(__dirname, 'types'),
        '~': path.resolve(__dirname, 'src/modules'),
      },
    },
    css: {
      preprocessorOptions: {
        scss: {
          api: 'modern-compiler',
          // additionalData: scssFiles.join(''),
          javascriptEnabled: true,
        },
      },
    },
    optimizeDeps: { include, exclude },
  })
}
