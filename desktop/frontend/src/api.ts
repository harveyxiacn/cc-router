export interface UsageWindow { usedPercentage: number; resetsAt: number }
export interface UsageSnapshot {
  observedAt: string
  fiveHour: UsageWindow | null
  sevenDay: UsageWindow | null
  source: 'official-statusline'
}
export interface ManualInput { fiveHour: UsageWindow | null; sevenDay: UsageWindow | null; limitedUntil: number }
export interface ManualRecord extends ManualInput { observedAt: string; source: 'manual' }
export interface BackupInfo { id: string; createdAt: string; reason: string; accountCount: number; bindingCount: number; restorable: boolean; error?: string }
export interface BackupPreview { info: BackupInfo; accounts: { id: string; name: string; label: string }[]; defaultAccountId: string; projectBindings: Record<string, string>; digest: string }
export interface Account { id: string; name: string; label: string; isDefault: boolean; active: boolean; usage: UsageSnapshot | null; usageError?: string; manualUsage?: ManualRecord | null; manualUsageError?: string }
export interface Snapshot { accounts: Account[]; project: string; boundAccount: string; dataDir: string }
export interface Identity { known: boolean; loggedIn: boolean; email: string; authMethod: string; version: string }
export interface Finding { source: string; key: string; message: string; blocking: boolean }
export interface Diagnosis { version: string; findings: Finding[]; error: string }
export interface Handoff { path: string; content: string; digest: string }
export interface UpdateInfo {
  currentVersion: string; latestVersion: string; releaseURL: string; notes: string; checkedAt: string
  updateAvailable: boolean; prerelease: boolean; autoUpdate: boolean; phase: string; progress: number
  message: string; error: string; canRollback: boolean; rollbackVersion: string
}

export interface AppBinding {
  GetSnapshot(project: string): Promise<Snapshot>
  ChooseDirectory(): Promise<string>
  CreateAccount(name: string, label: string): Promise<void>
  RenameAccount(name: string, newName: string, label: string): Promise<void>
  RemoveAccount(name: string): Promise<void>
  SetDefault(name: string): Promise<void>
  Bind(project: string, name: string): Promise<void>
  Unbind(project: string): Promise<void>
  CheckAccount(name: string, project: string): Promise<Identity>
  Diagnose(name: string, project: string): Promise<Diagnosis>
  CreateHandoff(project: string): Promise<Handoff>
  ReadHandoff(project: string): Promise<Handoff>
  SaveHandoff(project: string, content: string, expectedDigest: string): Promise<Handoff>
  Launch(name: string, project: string, mode: 'run' | 'login' | 'switch', reviewed: boolean): Promise<void>
  InstallUsage(name: string, prepare: number, switchAt: number): Promise<void>
  RecordManualUsage(name: string, input: ManualInput): Promise<void>
  ClearManualUsage(name: string): Promise<void>
  ListBackups(): Promise<BackupInfo[]>
  CreateBackup(): Promise<BackupInfo>
  PreviewBackup(id: string): Promise<BackupPreview>
  RestoreBackup(id: string, digest: string, reviewed: boolean): Promise<BackupInfo>
  ExportMetadata(): Promise<string>
  ImportMetadata(): Promise<string>
  CheckForUpdates(): Promise<UpdateInfo>
  GetUpdateStatus(): Promise<UpdateInfo>
  SetAutoUpdate(enabled: boolean): Promise<void>
  SetUpdateIdle(idle: boolean): Promise<void>
  PrepareUpdate(): Promise<void>
  ApplyUpdate(): Promise<void>
  RollbackUpdate(): Promise<void>
  ReportReady(): Promise<void>
  OpenReleasePage(): Promise<void>
}
declare global { interface Window { go?: { main?: { App?: AppBinding } } } }

export function backend(): AppBinding {
  const binding = window.go?.main?.App
  if (!binding) throw new Error('桌面连接尚未就绪。请从 CC Router 桌面应用打开；浏览器预览不连接账号数据。')
  return binding
}
