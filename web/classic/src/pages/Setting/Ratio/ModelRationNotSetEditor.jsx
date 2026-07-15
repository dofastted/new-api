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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Banner, Button, Space, Spin } from '@douyinfe/semi-ui';
import { API, showError } from '../../../helpers';
import { useTranslation } from 'react-i18next';
import ModelPricingEditor from './components/ModelPricingEditor';
import {
  buildCanonicalPricingOptions,
  fetchModelPricing,
} from './modelPricingApi';

export default function ModelRatioNotSetEditor(props) {
  const { t } = useTranslation();
  const [enabledModels, setEnabledModels] = useState([]);
  const [pricingViews, setPricingViews] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const loadData = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [modelsResponse, views] = await Promise.all([
        API.get('/api/channel/models_enabled'),
        fetchModelPricing(),
      ]);
      const { success, message, data } = modelsResponse.data;
      if (!success) {
        throw new Error(message || t('获取启用模型失败'));
      }
      setEnabledModels(data);
      setPricingViews(views);
    } catch (loadError) {
      setPricingViews(null);
      setError(loadError.message || t('获取启用模型失败'));
      showError(loadError.message || t('获取启用模型失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const canonicalOptions = useMemo(
    () => buildCanonicalPricingOptions(pricingViews || [], props.options),
    [pricingViews, props.options],
  );

  const refreshPricing = useCallback(async () => {
    await Promise.all([loadData(), props.refresh?.()]);
  }, [loadData, props.refresh]);
  if (error) {
    return (
      <Space vertical align='start'>
        <Banner type='danger' description={error} />
        <Button onClick={loadData} loading={loading}>
          {t('重试')}
        </Button>
      </Space>
    );
  }

  if (!pricingViews) return <Spin spinning={loading} />;

  return (
    <ModelPricingEditor
      options={canonicalOptions}
      pricingViews={pricingViews}
      refresh={refreshPricing}
      candidateModelNames={enabledModels}
      filterMode='unset'
      allowAddModel={false}
      allowDeleteModel={false}
      showConflictFilter={false}
      listDescription={t(
        '此页面仅显示未设置价格或基础倍率的模型，设置后会自动从列表中移出',
      )}
      emptyTitle={t('没有未设置定价的模型')}
      emptyDescription={t('当前没有未设置定价的模型')}
    />
  );
}
