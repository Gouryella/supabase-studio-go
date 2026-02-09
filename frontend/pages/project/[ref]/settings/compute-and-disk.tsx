import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'

import { useParams } from 'common'
import { InputPostTab } from 'components/interfaces/DiskManagement/ui/InputPostTab'
import { DiskManagementForm } from 'components/interfaces/DiskManagement/DiskManagementForm'
import AlertError from 'components/ui/AlertError'
import { DocsButton } from 'components/ui/DocsButton'
import DefaultLayout from 'components/layouts/DefaultLayout'
import SettingsLayout from 'components/layouts/ProjectSettingsLayout/SettingsLayout'
import {
  ScaffoldContainer,
  ScaffoldDescription,
  ScaffoldHeader,
  ScaffoldTitle,
} from 'components/layouts/Scaffold'
import { useProjectDiskResizeMutation } from 'data/config/project-disk-resize-mutation'
import { projectKeys } from 'data/projects/keys'
import { executeSql } from 'data/sql/execute-sql-query'
import { useSelectedProjectQuery } from 'hooks/misc/useSelectedProject'
import { DOCS_URL, GB, IS_PLATFORM } from 'lib/constants'
import type { NextPageWithLayout } from 'types'
import { Badge, Button, Card, CardContent, CardHeader, Input_Shadcn_, Skeleton } from 'ui'

const MIN_DISK_SIZE_GB = 1
const MAX_DISK_SIZE_GB = 65536

const LegendItem = ({ color, label }: { color: string; label: string }) => (
  <div className="flex items-center">
    <div className={`w-2 h-2 rounded-full mr-2 ${color}`} />
    <span>{label}</span>
  </div>
)

const SelfHostedComputeAndDisk = () => {
  const { ref } = useParams()
  const queryClient = useQueryClient()
  const { data: project } = useSelectedProjectQuery()
  const currentDiskSizeGb = project?.volumeSizeGb ?? 8
  const [draftDiskSizeGb, setDraftDiskSizeGb] = useState(currentDiskSizeGb)

  useEffect(() => {
    setDraftDiskSizeGb(currentDiskSizeGb)
  }, [currentDiskSizeGb])

  const {
    data: diskDatabaseBreakdown,
    isPending: isLoadingDatabaseSize,
    isError: isDatabaseSizeError,
    error: databaseSizeError,
  } = useQuery({
    queryKey: ['project', ref, 'database-system-size'],
    queryFn: async ({ signal }) => {
      if (!ref) return { databaseSizeBytes: 0, systemSizeBytes: 0 }

      const { result } = await executeSql(
        {
          projectRef: ref,
          connectionString: project?.connectionString,
          sql: `
            select
              coalesce(sum(pg_database_size(datname)) filter (where datname not in ('template0', 'template1')), 0)::bigint as database_size_bytes,
              coalesce(sum(pg_database_size(datname)) filter (where datname in ('template0', 'template1')), 0)::bigint as system_size_bytes
            from pg_database;
          `,
          queryKey: ['disk-database-system-size'],
        },
        signal
      )

      const databaseSize = Number(result?.[0]?.database_size_bytes)
      const systemSize = Number(result?.[0]?.system_size_bytes)

      if (!Number.isFinite(databaseSize) || !Number.isFinite(systemSize)) {
        throw new Error('Failed to load disk size breakdown')
      }

      return { databaseSizeBytes: databaseSize, systemSizeBytes: systemSize }
    },
    enabled: typeof ref !== 'undefined',
    staleTime: 30 * 1000,
  })

  const { data: walSizeBytes = 0 } = useQuery({
    queryKey: ['project', ref, 'wal-size'],
    queryFn: async ({ signal }) => {
      if (!ref) return 0
      try {
        const { result } = await executeSql(
          {
            projectRef: ref,
            connectionString: project?.connectionString,
            sql: 'select coalesce(sum(size), 0)::bigint as wal_size_bytes from pg_ls_waldir();',
            queryKey: ['disk-wal-size'],
          },
          signal
        )
        const value = result?.[0]?.wal_size_bytes
        if (typeof value === 'number') return value

        const parsed = Number(value)
        return Number.isFinite(parsed) ? parsed : 0
      } catch {
        return 0
      }
    },
    enabled: typeof ref !== 'undefined',
    staleTime: 30 * 1000,
  })

  const { mutate: resizeProjectDisk, isPending: isResizingDisk } = useProjectDiskResizeMutation({
    onSuccess: async (_, variables) => {
      if (!ref) return

      queryClient.setQueriesData(
        { queryKey: projectKeys.detail(ref) },
        (previous: Record<string, any> | undefined) => {
          if (!previous) return previous
          return { ...previous, volumeSizeGb: variables.volumeSize }
        }
      )
      await queryClient.invalidateQueries({ queryKey: projectKeys.detail(ref) })
      setDraftDiskSizeGb(variables.volumeSize)
      toast.success(`Disk size updated to ${variables.volumeSize} GB`)
    },
  })

  const hasPendingChange = draftDiskSizeGb !== currentDiskSizeGb
  const isDraftInvalid =
    !Number.isInteger(draftDiskSizeGb) ||
    draftDiskSizeGb < MIN_DISK_SIZE_GB ||
    draftDiskSizeGb > MAX_DISK_SIZE_GB
  const displayDiskSizeGb = isDraftInvalid ? currentDiskSizeGb : draftDiskSizeGb

  const databaseSizeBytes = diskDatabaseBreakdown?.databaseSizeBytes ?? 0
  const systemSizeBytes = diskDatabaseBreakdown?.systemSizeBytes ?? 0

  const databaseSizeGb = useMemo(() => databaseSizeBytes / GB, [databaseSizeBytes])
  const walSizeGb = useMemo(() => walSizeBytes / GB, [walSizeBytes])
  const systemSizeGb = useMemo(() => systemSizeBytes / GB, [systemSizeBytes])
  const usedSizeGb = useMemo(
    () => Math.min(displayDiskSizeGb, databaseSizeGb + walSizeGb + systemSizeGb),
    [displayDiskSizeGb, databaseSizeGb, walSizeGb, systemSizeGb]
  )
  const availableSizeGb = Math.max(displayDiskSizeGb - usedSizeGb, 0)

  const toPercent = (valueGb: number) => {
    if (displayDiskSizeGb <= 0) return 0
    return Math.min((valueGb / displayDiskSizeGb) * 100, 100)
  }

  const databasePercentage = toPercent(databaseSizeGb)
  const walPercentage = toPercent(walSizeGb)
  const systemPercentage = toPercent(systemSizeGb)
  const availablePercentage = Math.max(
    0,
    100 - databasePercentage - walPercentage - systemPercentage
  )

  const onSave = () => {
    if (!ref || isDraftInvalid) return
    resizeProjectDisk({ projectRef: ref, volumeSize: draftDiskSizeGb })
  }

  const onReset = () => setDraftDiskSizeGb(currentDiskSizeGb)

  return (
    <ScaffoldContainer>
      <Card>
        <CardHeader className="flex-row items-center justify-between">
          Disk Size
          {hasPendingChange && !isDraftInvalid && <Badge variant="success">New disk size</Badge>}
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid @xl:grid-cols-12 gap-5">
            <div className="col-span-4 space-y-4">
              <p className="text-sm text-foreground">Disk Size</p>
              <div className="flex items-center gap-2">
                <InputPostTab label="GB">
                  <Input_Shadcn_
                    type="number"
                    min={MIN_DISK_SIZE_GB}
                    max={MAX_DISK_SIZE_GB}
                    value={draftDiskSizeGb}
                    onWheel={(event) => event.currentTarget.blur()}
                    onChange={(event) => {
                      const value = Math.trunc(event.target.valueAsNumber)
                      if (Number.isNaN(value)) {
                        setDraftDiskSizeGb(0)
                        return
                      }
                      setDraftDiskSizeGb(value)
                    }}
                    className="w-32 rounded-r-none font-mono"
                  />
                </InputPostTab>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  type="primary"
                  disabled={!hasPendingChange || isDraftInvalid || isResizingDisk}
                  loading={isResizingDisk}
                  onClick={onSave}
                >
                  Save
                </Button>
                <Button
                  type="default"
                  disabled={!hasPendingChange || isResizingDisk}
                  onClick={onReset}
                >
                  Reset
                </Button>
              </div>
              <DocsButton abbrev={false} href={`${DOCS_URL}/guides/platform/database-size`} />
              {isDraftInvalid && (
                <p className="text-sm text-warning">
                  Disk size must be between {MIN_DISK_SIZE_GB} and {MAX_DISK_SIZE_GB} GB.
                </p>
              )}
            </div>

            <div className="col-span-8 space-y-3">
              <div className="h-6 flex items-center gap-3">
                {isLoadingDatabaseSize ? (
                  <Skeleton className="h-5 w-40" />
                ) : (
                  <span className="text-foreground-light text-sm font-mono">
                    {usedSizeGb.toFixed(2)} GB used of{' '}
                    <span className="text-foreground font-semibold">{displayDiskSizeGb} GB</span>
                  </span>
                )}
              </div>
              <div className="relative">
                <div className="h-[35px] relative border rounded-sm w-full overflow-hidden bg-surface-300">
                  <div className="h-full flex">
                    <div className="bg-foreground" style={{ width: `${databasePercentage}%` }} />
                    <div className="bg-_secondary" style={{ width: `${walPercentage}%` }} />
                    <div className="bg-destructive-500" style={{ width: `${systemPercentage}%` }} />
                    <div className="bg-border" style={{ width: `${availablePercentage}%` }} />
                  </div>
                </div>
              </div>
              <div className="flex items-center space-x-3 text-xs text-foreground-lighter">
                <LegendItem color="bg-foreground" label="Database" />
                <LegendItem color="bg-_secondary" label="WAL" />
                <LegendItem color="bg-destructive-500" label="System" />
                <LegendItem color="bg-border" label="Available space" />
              </div>
              <p className="text-xs text-foreground-lighter mt-3">
                <span className="font-semibold">Note:</span> Disk Size refers to the total space
                your project occupies on disk, including the database itself (currently{' '}
                {databaseSizeGb.toFixed(2)} GB), additional files like the write-ahead log
                (currently {walSizeGb.toFixed(2)} GB), and system resources (currently{' '}
                {systemSizeGb.toFixed(2)} GB).
              </p>
            </div>
          </div>

          {isDatabaseSizeError && (
            <AlertError error={databaseSizeError} subject="Failed to load current database size" />
          )}
        </CardContent>
      </Card>
    </ScaffoldContainer>
  )
}

const AuthSettings: NextPageWithLayout = () => {
  return (
    <>
      <ScaffoldContainer>
        <ScaffoldHeader>
          <ScaffoldTitle>Compute and Disk</ScaffoldTitle>
          <ScaffoldDescription>
            Configure the compute and disk settings for your project.
          </ScaffoldDescription>
        </ScaffoldHeader>
      </ScaffoldContainer>
      {IS_PLATFORM ? <DiskManagementForm /> : <SelfHostedComputeAndDisk />}
    </>
  )
}

AuthSettings.getLayout = (page) => (
  <DefaultLayout>
    <SettingsLayout>{page}</SettingsLayout>
  </DefaultLayout>
)
export default AuthSettings
