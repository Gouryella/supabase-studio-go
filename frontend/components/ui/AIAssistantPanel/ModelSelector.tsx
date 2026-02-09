import { Check, ChevronsUpDown } from 'lucide-react'
import { useMemo, useState } from 'react'

import { useIsFeatureEnabled } from 'hooks/misc/useIsFeatureEnabled'
import { useSelectedOrganizationQuery } from 'hooks/misc/useSelectedOrganization'
import { useRouter } from 'next/router'
import { IS_PLATFORM } from 'lib/constants'
import {
  Badge,
  Button,
  CommandGroup_Shadcn_,
  CommandItem_Shadcn_,
  CommandList_Shadcn_,
  Command_Shadcn_,
  PopoverContent_Shadcn_,
  PopoverTrigger_Shadcn_,
  Popover_Shadcn_,
  TooltipContent,
  TooltipTrigger,
  Tooltip,
} from 'ui'

interface ModelSelectorProps {
  models?: string[]
  selectedModel: string
  onSelectModel: (model: string) => void
}

export const ModelSelector = ({ models, selectedModel, onSelectModel }: ModelSelectorProps) => {
  const router = useRouter()
  const { data: organization } = useSelectedOrganizationQuery()

  const [open, setOpen] = useState(false)

  const canAccessProModels = organization?.plan?.id !== 'free'
  const slug = organization?.slug ?? '_'

  const upgradeHref = `/org/${slug ?? '_'}/billing?panel=subscriptionPlan&source=ai-assistant-model`

  const modelOptions = useMemo(() => {
    const normalized = (models ?? [])
      .map((model) => model?.trim())
      .filter((model): model is string => Boolean(model))
    if (normalized.length === 0) {
      return ['gpt-5-mini', 'gpt-5']
    }
    return Array.from(new Set(normalized))
  }, [models])

  const handleSelectModel = (model: string) => {
    if (IS_PLATFORM && model === 'gpt-5' && !canAccessProModels) {
      setOpen(false)
      void router.push(upgradeHref)
      return
    }

    onSelectModel(model)
    setOpen(false)
  }

  return (
    <Popover_Shadcn_ open={open} onOpenChange={setOpen}>
      <PopoverTrigger_Shadcn_ asChild>
        <Button
          type="default"
          className="text-foreground-light"
          iconRight={<ChevronsUpDown strokeWidth={1} size={12} />}
        >
          {selectedModel}
        </Button>
      </PopoverTrigger_Shadcn_>
      <PopoverContent_Shadcn_ className="p-0 w-44" align="start" side="top">
        <Command_Shadcn_>
          <CommandList_Shadcn_>
            <CommandGroup_Shadcn_>
              {modelOptions.map((model) => {
                const isGpt5 = model === 'gpt-5'
                const showUpgrade = IS_PLATFORM && isGpt5 && !canAccessProModels

                return (
                  <CommandItem_Shadcn_
                    key={model}
                    value={model}
                    onSelect={() => handleSelectModel(model)}
                    className="flex justify-between"
                  >
                    <span>{model}</span>
                    {showUpgrade ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <div>
                            <Badge role="button" variant="warning">
                              Upgrade
                            </Badge>
                          </div>
                        </TooltipTrigger>
                        <TooltipContent side="right">
                          gpt-5 is available on Pro plans and above
                        </TooltipContent>
                      </Tooltip>
                    ) : selectedModel === model ? (
                      <Check className="h-3.5 w-3.5" />
                    ) : null}
                  </CommandItem_Shadcn_>
                )
              })}
            </CommandGroup_Shadcn_>
          </CommandList_Shadcn_>
        </Command_Shadcn_>
      </PopoverContent_Shadcn_>
    </Popover_Shadcn_>
  )
}
