import type { ResponseStruct } from '#/global'

export interface SensitiveApprovalVo {
  approval_id: string
  requester_id: number
  approver_id: number
  tenant_id: number
  policy_key: string
  binding_digest: string
  scope: string
  resource: string
  status: 'pending' | 'approved' | 'rejected'
  reason: string
  used_at?: string
  expires_at?: string
}

export interface ReAuthTokenVo {
  reauth_token: string
  expires_at: string
}

export interface SensitiveEvidenceRequest {
  policy_key?: string
  scope: string
  resource: string
  reason: string
  approval_required?: boolean
}

export interface SensitiveEvidenceResult {
  reauth_token: string
  approval_id?: string
}

export function issueReAuthToken(data: {
  password: string
  mfa_code?: string
  operation: string
  resource: string
}): Promise<ResponseStruct<ReAuthTokenVo>> {
  return useHttp().post(securityControlPath('/reauth-token'), data)
}

export function createApproval(data: {
  policy_key: string
  resource: string
  reason: string
}): Promise<ResponseStruct<SensitiveApprovalVo>> {
  return useHttp().post(securityControlPath('/approvals'), data)
}

export function approvalDetail(approvalID: string): Promise<ResponseStruct<SensitiveApprovalVo>> {
  return useHttp().get(securityControlPath(`/approvals/${encodeURIComponent(approvalID)}`))
}

export function approveApproval(approvalID: string): Promise<ResponseStruct<SensitiveApprovalVo>> {
  return useHttp().put(securityControlPath(`/approvals/${encodeURIComponent(approvalID)}/approve`))
}

function securityControlPath(path: string) {
  const prefix = useUserStore().authScope === 'platform' ? '/admin/platform/security' : '/admin/security'
  return `${prefix}${path}`
}
