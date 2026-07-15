/*
Copyright (C) 2025 QuantumNous

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
import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Modal,
  Select,
  Space,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { showError, showSuccess } from '../../../helpers';
import { saveModelPricingBatch } from './modelPricingApi';

const { Text } = Typography;

const PRICING_FIELDS = [
  ['mode', '计费方式'],
  ['price', '固定价格'],
  ['ratio', '输入倍率'],
  ['completion_ratio', '输出倍率'],
  ['cache_ratio', '缓存倍率'],
  ['create_cache_ratio', '缓存创建倍率'],
  ['image_ratio', '图片倍率'],
  ['audio_ratio', '音频输入倍率'],
  ['audio_completion_ratio', '音频输出倍率'],
  ['billing_expr', '计费表达式'],
];

const pricingValue = (config, key) => {
  const value = config?.[key];
  return value === undefined || value === '' ? null : String(value);
};

export default function OfficialPricingConflictReview({ conflicts, refresh }) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  const [selectedModel, setSelectedModel] = useState('');
  const [submitting, setSubmitting] = useState('');
  const selected =
    conflicts.find((view) => view.model_name === selectedModel) || conflicts[0];

  useEffect(() => {
    if (conflicts.length === 0) {
      setVisible(false);
      setSelectedModel('');
      return;
    }
    if (!conflicts.some((view) => view.model_name === selectedModel)) {
      setSelectedModel(conflicts[0].model_name);
    }
  }, [conflicts, selectedModel]);

  const differences = useMemo(() => {
    if (!selected?.manual_config || !selected?.official_config) return [];
    return PRICING_FIELDS.flatMap(([key, label]) => {
      const manual = pricingValue(selected.manual_config, key);
      const official = pricingValue(selected.official_config, key);
      if (manual === official) return [];
      return [{ key, label, manual, official }];
    });
  }, [selected]);

  if (!selected) return null;

  const handleChoice = async (choice) => {
    setSubmitting(choice);
    try {
      if (choice === 'manual') {
        if (!selected.official_config_hash) {
          throw new Error(t('官方价格不可用'));
        }
        await saveModelPricingBatch({
          upserts: [],
          restore: [],
          acknowledge: [
            {
              model_name: selected.authority_model_name,
              official_config_hash: selected.official_config_hash,
            },
          ],
        });
        showSuccess(t('已保留手动价格'));
      } else {
        await saveModelPricingBatch({
          upserts: [],
          restore: [selected.authority_model_name],
        });
        showSuccess(t('已恢复官方价格'));
      }
      setVisible(false);
      await refresh();
    } catch (error) {
      showError(error.message || t('处理价格冲突失败'));
    } finally {
      setSubmitting('');
    }
  };

  return (
    <>
      <Banner
        type='warning'
        description={
          <Space wrap>
            <Text>
              {t('{{count}} 个手动模型价格与最新官方价格不同。', {
                count: conflicts.length,
              })}
            </Text>
            <Button size='small' onClick={() => setVisible(true)}>
              {t('审核价格变化')}
            </Button>
          </Space>
        }
      />

      <Modal
        title={t('审核官方价格变化')}
        visible={visible}
        onCancel={() => setVisible(false)}
        width={860}
        footer={
          <Space>
            <Button
              loading={submitting === 'manual'}
              disabled={Boolean(submitting)}
              onClick={() => handleChoice('manual')}
            >
              {t('保留手动价格')}
            </Button>
            <Button
              type='danger'
              loading={submitting === 'official'}
              disabled={Boolean(submitting)}
              onClick={() => handleChoice('official')}
            >
              {t('使用官方价格')}
            </Button>
          </Space>
        }
      >
        <Space vertical align='start' style={{ width: '100%' }}>
          <Text>{t('在完成选择前，系统继续使用手动价格计费。')}</Text>
          {conflicts.length > 1 ? (
            <Select
              value={selected.model_name}
              onChange={setSelectedModel}
              style={{ minWidth: 280 }}
              optionList={conflicts.map((view) => ({
                label: view.model_name,
                value: view.model_name,
              }))}
            />
          ) : (
            <Text strong code>
              {selected.model_name}
            </Text>
          )}
          <Table
            pagination={false}
            size='small'
            dataSource={differences}
            rowKey='key'
            columns={[
              {
                title: t('价格字段'),
                dataIndex: 'label',
                render: (value) => t(value),
              },
              {
                title: t('手动值'),
                dataIndex: 'manual',
                render: (value) => <Text code>{value ?? t('无值')}</Text>,
              },
              {
                title: t('官方值'),
                dataIndex: 'official',
                render: (value) => <Text code>{value ?? t('无值')}</Text>,
              },
            ]}
          />
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 1fr',
              gap: 12,
              width: '100%',
            }}
          >
            <div>
              <Text strong>{t('完整手动配置')}</Text>
              <pre
                style={{
                  maxHeight: 220,
                  overflow: 'auto',
                  padding: 12,
                  background: 'var(--semi-color-fill-0)',
                }}
              >
                {JSON.stringify(selected.manual_config, null, 2)}
              </pre>
            </div>
            <div>
              <Text strong>{t('完整官方配置')}</Text>
              <pre
                style={{
                  maxHeight: 220,
                  overflow: 'auto',
                  padding: 12,
                  background: 'var(--semi-color-fill-0)',
                }}
              >
                {JSON.stringify(selected.official_config, null, 2)}
              </pre>
            </div>
          </div>
        </Space>
      </Modal>
    </>
  );
}
