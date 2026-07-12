import type {
  SensitiveEvidenceRequest,
  SensitiveEvidenceResult,
} from '~/base/api/platformSecurityControl'

export interface EvidenceDialogExpose {
  open: (request: SensitiveEvidenceRequest) => Promise<SensitiveEvidenceResult>
}

interface RequestSensitiveEvidenceOptions {
  dialog?: EvidenceDialogExpose
  request: SensitiveEvidenceRequest
}

interface ConfirmSensitiveOperationOptions {
  confirm: () => Promise<unknown>
  run: () => Promise<void>
}

export async function requestSensitiveEvidence(options: RequestSensitiveEvidenceOptions) {
  if (!options.dialog) {
    return null
  }
  try {
    return await options.dialog.open(options.request)
  }
  catch {
    return null
  }
}

export async function confirmSensitiveOperation(options: ConfirmSensitiveOperationOptions) {
  try {
    await options.confirm()
  }
  catch {
    return
  }
  await options.run()
}
