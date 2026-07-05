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
import { SettingsPage } from '../components/settings-page'
import type { SecuritySettings } from '../types'
import {
  SECURITY_DEFAULT_SECTION,
  getSecuritySectionContent,
  getSecuritySectionMeta,
} from './section-registry.tsx'

const defaultAbuseGuardBlockWords = [
  'steal api key',
  'steal cookies',
  'dump saved passwords',
  'extract oauth token',
  'extract refresh token',
  'session hijack',
  'bypass login',
  'account takeover',
  'write ransomware',
  'create ransomware',
  'build ransomware',
  'create keylogger',
  'write keylogger',
  'credential stealer',
  'cookie stealer',
  'undetectable malware',
  'bypass antivirus',
  'disable antivirus',
  'phishing kit',
  '窃取cookie',
  '盗取cookie',
  '抓取用户token',
  '导出用户token',
  '窃取密码',
  '盗取密码',
  '提取oauth',
  '劫持会话',
  '接管账号',
  '绕过登录',
  '生成勒索软件',
  '制作勒索软件',
  '编写键盘记录器',
  '绕过杀毒软件',
  '关闭杀毒软件',
  '免杀木马',
  '钓鱼网站源码',
]

const defaultSecuritySettings: SecuritySettings = {
  ModelRequestRateLimitEnabled: false,
  ModelRequestRateLimitCount: 0,
  ModelRequestRateLimitSuccessCount: 1000,
  ModelRequestRateLimitDurationMinutes: 1,
  ModelRequestRateLimitWaitEnabled: true,
  RateLimitWaitTimeoutSeconds: 60,
  RateLimitMaxWaitingPerUser: 10,
  ModelRequestRateLimitGroup: '',
  CheckSensitiveEnabled: false,
  CheckSensitiveOnPromptEnabled: false,
  SensitiveWords: '',
  'fetch_setting.enable_ssrf_protection': true,
  'fetch_setting.allow_private_ip': false,
  'fetch_setting.domain_filter_mode': false,
  'fetch_setting.ip_filter_mode': false,
  'fetch_setting.domain_list': [],
  'fetch_setting.ip_list': [],
  'fetch_setting.allowed_ports': [],
  'fetch_setting.apply_ip_filter_for_domain': false,
  'token_setting.max_user_tokens': 1000,
  'abuse_guard.enabled': false,
  'abuse_guard.monitor_only': false,
  'abuse_guard.model_scope_patterns': [],
  'abuse_guard.exempt_groups': [],
  'abuse_guard.block_words': defaultAbuseGuardBlockWords,
  'abuse_guard.disabled_builtin_ids': [],
  'abuse_guard.custom_patterns': '',
  'abuse_guard.pattern_block_score': 10,
  'abuse_guard.scan_window_kb': 32,
  'abuse_guard.moderation_api_key': '',
  'abuse_guard.moderation_base_url': 'https://api.openai.com',
  'abuse_guard.moderation_model': 'omni-moderation-latest',
  'abuse_guard.sample_rate_percent': 5,
  'abuse_guard.review_snippet_kb': 16,
  'abuse_guard.queue_size': 1024,
  'abuse_guard.worker_count': 4,
  'abuse_guard.category_scores': '',
  'abuse_guard.instant_ban_categories': [],
  'abuse_guard.score_window_hours': 24,
  'abuse_guard.ban_threshold': 5,
  'abuse_guard.temp_ban_hours': 24,
  'abuse_guard.perm_ban_after_temp_bans': 3,
}

export function SecuritySettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/security/$section'
      defaultSettings={defaultSecuritySettings}
      defaultSection={SECURITY_DEFAULT_SECTION}
      getSectionContent={getSecuritySectionContent}
      getSectionMeta={getSecuritySectionMeta}
    />
  )
}
