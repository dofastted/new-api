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
  Download,
  ExternalLink,
  KeyRound,
  MousePointerClick,
  Route,
} from 'lucide-react'
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

export function CCSwitchOnboardingGuide() {
  const { t } = useTranslation()
  const { setOpen } = useApiKeys()

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
    <Card className='border-primary/10 bg-primary/[0.025]'>
      <CardHeader className='gap-3 py-4 sm:flex-row sm:items-center sm:justify-between'>
        <div className='space-y-1'>
          <CardTitle className='text-base'>
            {t('Connect ccswitch quickly')}
          </CardTitle>
          <CardDescription className='max-w-3xl text-xs sm:text-sm'>
            {t(
              'Create an API key, choose a provider group, then click the ccswitch button beside the API key to import it.'
            )}
          </CardDescription>
        </div>
        <CardAction className='flex shrink-0 flex-wrap gap-2'>
          <Button
            size='sm'
            variant='outline'
            render={
              <a
                href={CC_SWITCH_DOWNLOAD_URL}
                target='_blank'
                rel='noreferrer'
              />
            }
          >
            <Download className='size-4' />
            {t('Download ccswitch')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            render={<a href={CLI_GUIDE_URL} />}
          >
            <ExternalLink className='size-4' />
            {t('View CLI guide')}
          </Button>
          <Button size='sm' onClick={() => setOpen('create')}>
            <MousePointerClick className='size-4' />
            {t('Create API Key')}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='pt-0'>
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
