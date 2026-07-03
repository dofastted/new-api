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
import { useQuery } from '@tanstack/react-query'
import {
  BookOpen,
  ExternalLink,
  KeyRound,
  MousePointerClick,
  TerminalSquare,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Markdown } from '@/components/ui/markdown'
import { Skeleton } from '@/components/ui/skeleton'

import { getAboutContent } from './api'

function isValidUrl(value: string) {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

function isLikelyHtml(value: string) {
  return /<\/?[a-z][\s\S]*>/i.test(value)
}

const CC_SWITCH_URL = 'https://ccswitch.io'
const CODEX_URL = 'https://github.com/openai/codex'
const CLAUDE_CODE_URL = 'https://code.claude.com/docs/en/setup'
const PI_URL = 'https://pi.dev/'
const OMP_URL = 'https://omp.sh/'
const OPENCODE_URL = 'https://opencode.ai/'

function EmptyAboutState() {
  const { t } = useTranslation()
  const currentYear = new Date().getFullYear()

  const setupFlow = [
    {
      id: 'install-ccswitch',
      icon: <TerminalSquare className='size-4' />,
      title: t('Install ccswitch and keep it running.'),
    },
    {
      id: 'install-cli',
      icon: <TerminalSquare className='size-4' />,
      title: t(
        'Install the CLI you want to use, such as Codex or Claude Code.'
      ),
    },
    {
      id: 'create-key',
      icon: <KeyRound className='size-4' />,
      title: t('Create or select an API key on the API Keys page.'),
    },
    {
      id: 'import-key',
      icon: <MousePointerClick className='size-4' />,
      title: t(
        'Click the ccswitch icon beside the key, choose the app and model, then open the import link.'
      ),
    },
  ]

  const cliTools = [
    {
      name: 'ccswitch',
      href: CC_SWITCH_URL,
      linkLabel: t('Official site'),
      description: t(
        'Desktop switcher for Claude Code, Codex, Gemini, and automatic balance viewing.'
      ),
      reference: 'ccswitch.io',
    },
    {
      name: 'Codex',
      href: CODEX_URL,
      linkLabel: t('GitHub'),
      description: t('OpenAI coding agent for your terminal.'),
      command: 'npm install -g @openai/codex',
    },
    {
      name: 'Claude Code',
      href: CLAUDE_CODE_URL,
      linkLabel: t('Official docs'),
      description: t('Anthropic coding assistant for local projects.'),
      command: 'curl -fsSL https://claude.ai/install.sh | bash',
    },
    {
      name: 'Pi',
      href: PI_URL,
      linkLabel: t('Official site'),
      description: t('Minimal extensible coding agent framework.'),
      reference: 'pi.dev',
    },
    {
      name: 'OMP',
      href: OMP_URL,
      linkLabel: t('Official site'),
      description: t(
        'Oh My Pi coding agent with terminal and IDE-style workflows.'
      ),
      reference: 'omp.sh',
    },
    {
      name: 'OpenCode',
      href: OPENCODE_URL,
      linkLabel: t('Official site'),
      description: t(
        'Open source AI coding agent for terminal, IDE, and desktop.'
      ),
      command: 'curl -fsSL https://opencode.ai/install | bash',
    },
  ]

  return (
    <div className='mx-auto max-w-5xl space-y-8 px-4 py-10'>
      <section
        id='ccswitch-guide'
        className='bg-card rounded-2xl border p-6 shadow-sm'
      >
        <div className='flex flex-col gap-4 md:flex-row md:items-start md:justify-between'>
          <div className='max-w-3xl space-y-2'>
            <div className='text-primary flex items-center gap-2 text-sm font-medium'>
              <BookOpen className='size-4' />
              {t('CLI connection guide')}
            </div>
            <h1 className='text-2xl font-bold tracking-tight'>
              {t('ccswitch setup flow')}
            </h1>
            <p className='text-muted-foreground'>
              {t(
                'Create an API key, choose its provider group, then use the ccswitch shortcut beside the API key to import it.'
              )}
            </p>
          </div>
          <a
            href={CC_SWITCH_URL}
            target='_blank'
            rel='noopener noreferrer'
            className='border-input bg-background hover:bg-accent hover:text-accent-foreground inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-md border px-4 text-sm font-medium transition-colors'
          >
            {t('Download ccswitch')}
            <ExternalLink className='size-4' />
          </a>
        </div>

        <div className='mt-6 grid gap-3 md:grid-cols-4'>
          {setupFlow.map((step) => (
            <div
              key={step.id}
              className='bg-background/70 rounded-xl border p-3'
            >
              <div className='bg-muted text-muted-foreground mb-3 flex size-8 items-center justify-center rounded-lg'>
                {step.icon}
              </div>
              <p className='text-sm leading-5 font-medium'>{step.title}</p>
            </div>
          ))}
        </div>
      </section>

      <section id='cli-guide' className='space-y-4'>
        <div className='space-y-1'>
          <h2 className='text-xl font-semibold'>{t('Supported CLI tools')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Install the CLI from its official source, then let ccswitch write the provider settings from your API key.'
            )}
          </p>
        </div>
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {cliTools.map((tool) => (
            <article key={tool.name} className='bg-card rounded-xl border p-4'>
              <div className='flex items-start justify-between gap-3'>
                <div>
                  <h3 className='font-semibold'>{tool.name}</h3>
                  <p className='text-muted-foreground mt-1 text-sm leading-5'>
                    {tool.description}
                  </p>
                </div>
                <a
                  href={tool.href}
                  target='_blank'
                  rel='noopener noreferrer'
                  className='text-primary inline-flex shrink-0 items-center gap-1 text-xs font-medium hover:underline'
                >
                  {tool.linkLabel}
                  <ExternalLink className='size-3' />
                </a>
              </div>
              <div className='bg-muted/70 mt-3 rounded-lg px-3 py-2 text-xs'>
                {tool.command ? (
                  <>
                    <span className='text-muted-foreground'>
                      {t('Install command:')}
                    </span>{' '}
                    <code className='font-mono break-all'>{tool.command}</code>
                  </>
                ) : (
                  <>
                    <span className='text-muted-foreground'>
                      {t('Reference:')}
                    </span>{' '}
                    <code className='font-mono'>{tool.reference}</code>
                  </>
                )}
              </div>
            </article>
          ))}
        </div>
      </section>

      <section className='text-muted-foreground border-t pt-6 text-sm'>
        <h2 className='text-foreground mb-3 text-base font-semibold'>
          {t('Project and license')}
        </h2>
        <p>
          {t('New API Project Repository:')}{' '}
          <a
            href='https://github.com/QuantumNous/new-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('https://github.com/QuantumNous/new-api')}
          </a>
        </p>
        <p className='mt-2'>
          <a
            href='https://github.com/QuantumNous/new-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('NewAPI')}
          </a>{' '}
          © {currentYear}{' '}
          <a
            href='https://github.com/QuantumNous'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('QuantumNous')}
          </a>{' '}
          {t('| Based on')}{' '}
          <a
            href='https://github.com/songquanpeng/one-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('One API')}
          </a>{' '}
          © 2023{' '}
          <a
            href='https://github.com/songquanpeng'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('JustSong')}
          </a>
        </p>
        <p className='mt-2'>
          {t('This project must be used in compliance with the')}{' '}
          <a
            href='https://github.com/QuantumNous/new-api/blob/main/LICENSE'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('AGPL v3.0 License')}
          </a>
          .
        </p>
      </section>
    </div>
  )
}

export function About() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['about-content'],
    queryFn: getAboutContent,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isValidUrl(rawContent)
  const isHtml = hasContent && !isUrl && isLikelyHtml(rawContent)

  if (isLoading) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
          <Skeleton className='h-8 w-[45%]' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-[90%]' />
          <Skeleton className='h-4 w-[80%]' />
        </div>
      </PublicLayout>
    )
  }

  if (!hasContent) {
    return (
      <PublicLayout>
        <EmptyAboutState />
      </PublicLayout>
    )
  }

  if (isUrl) {
    return (
      <PublicLayout showMainContainer={false}>
        <iframe
          src={rawContent}
          className='h-[calc(100vh-3.5rem)] w-full border-0'
          title={t('About')}
          sandbox='allow-scripts allow-forms allow-popups allow-presentation'
        />
      </PublicLayout>
    )
  }

  return (
    <PublicLayout>
      <div className='mx-auto max-w-6xl px-4 py-8'>
        {isHtml ? (
          <div
            className='prose prose-neutral dark:prose-invert max-w-none'
            dangerouslySetInnerHTML={{ __html: rawContent }}
          />
        ) : (
          <Markdown className='prose-neutral dark:prose-invert max-w-none'>
            {rawContent}
          </Markdown>
        )}
      </div>
    </PublicLayout>
  )
}
