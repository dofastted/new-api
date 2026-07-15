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
import { useTranslation } from 'react-i18next';
import ModelPricingEditor from './components/ModelPricingEditor';
import {
  buildCanonicalPricingOptions,
  fetchModelPricing,
} from './modelPricingApi';

export default function ModelSettingsVisualEditor(props) {
  const { t } = useTranslation();
  const [pricingViews, setPricingViews] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const loadPricing = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setPricingViews(await fetchModelPricing());
    } catch (loadError) {
      setPricingViews(null);
      setError(loadError.message || t('加载模型价格失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    loadPricing();
  }, [loadPricing]);

  const options = useMemo(
    () => buildCanonicalPricingOptions(pricingViews || [], props.options),
    [pricingViews, props.options],
  );
  const refresh = useCallback(async () => {
    await Promise.all([loadPricing(), props.refresh?.()]);
  }, [loadPricing, props.refresh]);

  if (error) {
    return (
      <Space vertical align='start'>
        <Banner type='danger' description={error} />
        <Button onClick={loadPricing} loading={loading}>
          {t('重试')}
        </Button>
      </Space>
    );
  }
  if (!pricingViews) return <Spin spinning={loading} />;

  return (
    <ModelPricingEditor
      options={options}
      pricingViews={pricingViews}
      refresh={refresh}
    />
  );
}
