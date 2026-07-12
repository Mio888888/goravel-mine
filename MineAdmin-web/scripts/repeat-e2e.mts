import { mkdir, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { resolve } from 'node:path'
import { spawn } from 'node:child_process'
import process from 'node:process'

export interface E2ERepeatOptions {
  command: string[]
  iterations: number
  artifactRoot: string
}

interface RepeatArguments {
  gate: string
  iterations: number
  projects: string[]
  workers: string
  specs: string[]
}

interface StabilityIteration {
  iteration: number
  project: string
  status: 'passed' | 'failed'
  failedUrl?: string
  tracePath?: string
}

interface StabilityReport {
  schema: 'mock-e2e-stability/v1'
  gate: string
  command: string[]
  iterations: StabilityIteration[]
}

const artifactRoot = resolve('tests/e2e/.output')
const args = parseArguments(process.argv.slice(2))
const command = ['yarn', 'playwright', 'test', ...args.specs, `--workers=${args.workers}`, '--retries=0', '--max-failures=1']
const options: E2ERepeatOptions = { command, iterations: args.iterations, artifactRoot }
const report: StabilityReport = { schema: 'mock-e2e-stability/v1', gate: args.gate, command, iterations: [] }
const e2ePort = Number.parseInt(process.env.E2E_PORT || '2890', 10)

await mkdir(options.artifactRoot, { recursive: true })

for (const project of args.projects) {
  for (let iteration = 1; iteration <= options.iterations; iteration++) {
    const artifactDir = resolve(options.artifactRoot, 'repeat', args.gate, project, `iteration-${iteration}`)
    const outputDir = resolve(artifactDir, 'test-results')
    await rm(artifactDir, { recursive: true, force: true })
    await waitForPortRelease(e2ePort)
    const result = await run([
      ...options.command,
      `--project=${project}`,
      `--output=${outputDir}`,
    ], {
      E2E_PORT: String(e2ePort),
      E2E_REPORT_DIR: resolve(artifactDir, 'html-report'),
    })
    await waitForPortRelease(e2ePort)
    const entry: StabilityIteration = {
      iteration,
      project,
      status: result.exitCode === 0 ? 'passed' : 'failed',
    }
    if (entry.status === 'failed') {
      Object.assign(entry, await failureDetails(outputDir, result.output))
    }
    report.iterations.push(entry)
    await writeReport(options.artifactRoot, args.gate, report)
    if (entry.status === 'failed') {
      process.exitCode = result.exitCode || 1
      break
    }
  }
  if (process.exitCode) {
    break
  }
}

async function waitForPortRelease(port: number, timeoutMs = 15_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await canListen(port)) {
      return
    }
    await new Promise(resolveWait => setTimeout(resolveWait, 100))
  }
  throw new Error(`E2E port ${port} was not released within ${timeoutMs}ms`)
}

function canListen(port: number) {
  return new Promise<boolean>((resolveListen) => {
    const server = createServer()
    server.unref()
    server.once('error', () => resolveListen(false))
    server.listen(port, '127.0.0.1', () => server.close(() => resolveListen(true)))
  })
}

async function writeReport(root: string, gate: string, currentReport: StabilityReport) {
  await writeFile(resolve(root, `stability-report.${gate}.json`), `${JSON.stringify(currentReport, null, 2)}\n`)
}

async function failureDetails(outputDir: string, output: string) {
  const trace = output.match(/([\w./-]*\/trace\.zip)/)?.[1]
  const failedUrl = output.match(/(?:failed URL|url):\s*(\S+)/i)?.[1]
  return {
    ...(failedUrl ? { failedUrl } : {}),
    ...(trace ? { tracePath: resolve(trace) } : { tracePath: outputDir }),
  }
}

function run(command: string[], environment: NodeJS.ProcessEnv) {
  return new Promise<{ exitCode: number, output: string }>((resolveRun) => {
    const child = spawn(command[0], command.slice(1), {
      env: { ...process.env, ...environment },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let output = ''
    child.stdout.on('data', (chunk) => {
      const text = String(chunk)
      output += text
      process.stdout.write(text)
    })
    child.stderr.on('data', (chunk) => {
      const text = String(chunk)
      output += text
      process.stderr.write(text)
    })
    child.on('close', exitCode => resolveRun({ exitCode: exitCode ?? 1, output }))
  })
}

function parseArguments(values: string[]): RepeatArguments {
  const options: RepeatArguments = { gate: 'manual', iterations: 1, projects: [], workers: '1', specs: [] }
  for (const value of values) {
    if (value.startsWith('--gate=')) {
      options.gate = gateName(value.slice('--gate='.length))
    }
    else if (value.startsWith('--iterations=')) {
      options.iterations = positiveInteger(value, '--iterations=')
    }
    else if (value.startsWith('--project=')) {
      options.projects.push(value.slice('--project='.length))
    }
    else if (value.startsWith('--workers=')) {
      options.workers = String(positiveInteger(value, '--workers='))
    }
    else if (value.startsWith('--spec=')) {
      options.specs.push(value.slice('--spec='.length))
    }
    else {
      throw new Error(`Unsupported repeat E2E option: ${value}`)
    }
  }
  if (options.projects.length === 0) {
    throw new Error('At least one --project is required')
  }
  return options
}

function positiveInteger(value: string, prefix: string) {
  const parsed = Number.parseInt(value.slice(prefix.length), 10)
  if (!Number.isSafeInteger(parsed) || parsed < 1) {
    throw new Error(`${prefix.slice(0, -1)} must be a positive integer`)
  }
  return parsed
}

function gateName(value: string) {
  if (!/^[a-z0-9-]+$/i.test(value)) {
    throw new Error('--gate must contain only letters, numbers, and hyphens')
  }
  return value
}
