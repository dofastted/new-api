/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  ChevronDown,
  ChevronUp,
  Download,
  ExternalLink,
  KeyRound,
  MousePointerClick,
  Route,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import { useApiKeys } from './api-keys-provider'

const CC_SWITCH_DOWNLOAD_URL = 'https://ccswitch.io'
const CLI_GUIDE_URL = '/about#cli-guide'

const CC_SWITCH_ONBOARDING_VISIBILITY_STORAGE_KEY =
  'api_keys_ccswitch_onboarding_expanded'

function getSavedOnboardingExpanded(): boolean {
  if (typeof window === 'undefined') return true

  let saved: string | null = null
  try {
    saved = window.localStorage.getItem(
      CC_SWITCH_ONBOARDING_VISIBILITY_STORAGE_KEY
    )
  } catch {
    /* local storage may be unavailable */
  }

  if (saved === 'expanded') return true
  if (saved === 'collapsed') return false

  return !window.matchMedia('(max-width: 640px)').matches
}

function saveOnboardingExpanded(expanded: boolean): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(
      CC_SWITCH_ONBOARDING_VISIBILITY_STORAGE_KEY,
      expanded ? 'expanded' : 'collapsed'
    )
  } catch {
    /* local storage may be unavailable */
  }
}

export function CCSwitchOnboardingGuide() {
  const { t } = useTranslation()
  const { setOpen } = useApiKeys()
  const [expanded, setExpanded] = useState(getSavedOnboardingExpanded)

  const toggleExpanded = () => {
    const nextExpanded = !expanded
    setExpanded(nextExpanded)
    saveOnboardingExpanded(nextExpanded)
  }

  if (!expanded) {
    return (
      <Card size='sm' className='border-primary/10 bg-primary/[0.025] shrink-0'>
        <CardHeader className='grid-cols-[1fr_auto] items-center gap-2 py-2 sm:py-3'>
          <div className='min-w-0'>
            <CardTitle className='truncate text-sm'>
              {t('Connect ccswitch quickly')}
            </CardTitle>
          </div>
          <CardAction className='col-start-2 row-start-1 shrink-0 justify-self-end'>
            <Button
              size='sm'
              variant='outline'
              onClick={toggleExpanded}
              aria-label={t('Show setup guide')}
            >
              <ChevronDown className='size-4' aria-hidden='true' />
              <span className='max-sm:sr-only'>{t('Show setup guide')}</span>
            </Button>
          </CardAction>
        </CardHeader>
      </Card>
    )
  }
  const steps = [
    {
      id: 'key',
      icon: <KeyRound className='size-3.5' />,
      text: t('Create or select an API key.'),
    },
    {
      id: 'group',
      icon: <Route className='size-3.5' />,
      text: t('Choose the provider group for routing.'),
    },
    {
      id: 'ccswitch',
      icon: (
        <img
          src='/ccswitch-icon.png'
          alt=''
          className='size-4 rounded-sm object-contain'
        />
      ),
      text: t('Click the ccswitch button beside the key.'),
    },
  ]

  return (
    <Card size='sm' className='border-primary/10 bg-primary/[0.025] shrink-0'>
      <CardHeader className='gap-2 py-3 sm:gap-3 sm:py-4'>
        <div className='min-w-0 space-y-2'>
          <Button
            size='sm'
            variant='outline'
            onClick={toggleExpanded}
            className='w-fit'
          >
            <ChevronUp className='size-4' aria-hidden='true' />
            {t('Hide setup guide')}
          </Button>
          <div className='space-y-1'>
            <CardTitle className='text-sm sm:text-base'>
              {t('Connect ccswitch quickly')}
            </CardTitle>
            <CardDescription className='line-clamp-2 max-w-3xl text-xs sm:line-clamp-none sm:text-sm'>
              {t(
                'Create an API key, choose a provider group, then click the ccswitch button beside the API key to import it.'
              )}
            </CardDescription>
          </div>
        </div>
        <CardAction className='col-start-1 row-start-2 grid w-full grid-cols-3 gap-2 sm:col-start-2 sm:row-start-1 sm:flex sm:w-auto sm:flex-wrap sm:justify-end'>
          <Button
            size='sm'
            variant='outline'
            aria-label={t('Download ccswitch')}
            className='max-sm:w-full'
            render={
              <a
                href={CC_SWITCH_DOWNLOAD_URL}
                target='_blank'
                rel='noreferrer'
              />
            }
          >
            <Download className='size-4' aria-hidden='true' />
            <span className='max-sm:sr-only'>{t('Download ccswitch')}</span>
          </Button>
          <Button
            size='sm'
            variant='outline'
            aria-label={t('View CLI guide')}
            className='max-sm:w-full'
            render={<a href={CLI_GUIDE_URL} />}
          >
            <ExternalLink className='size-4' aria-hidden='true' />
            <span className='max-sm:sr-only'>{t('View CLI guide')}</span>
          </Button>
          <Button
            size='sm'
            onClick={() => setOpen('create')}
            aria-label={t('Create API Key')}
            className='max-sm:w-full'
          >
            <MousePointerClick className='size-4' aria-hidden='true' />
            <span className='max-sm:sr-only'>{t('Create API Key')}</span>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='hidden pt-0 sm:block'>
        <div className='text-muted-foreground grid gap-2 text-xs md:grid-cols-3'>
          {steps.map((step) => (
            <div
              key={step.id}
              className='bg-background/60 flex items-center gap-2 rounded-lg border px-2.5 py-2'
            >
              <span className='bg-muted text-muted-foreground flex size-7 shrink-0 items-center justify-center rounded-md'>
                {step.icon}
              </span>
              <span>{step.text}</span>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
