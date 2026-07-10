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

import React, { useEffect, useState, useRef } from 'react';
import {
  Button,
  Col,
  Form,
  Popconfirm,
  Row,
  Space,
  Spin,
} from '@douyinfe/semi-ui';
import {
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import {
  buildPricingViewMap,
  calibrateModelPricing,
  pricingConfigSignature,
  saveModelPricingBatch,
} from './modelPricingApi';
import { useTranslation } from 'react-i18next';

const parseJsonMap = (value) => JSON.parse(value || '{}');

const buildPricingConfigs = (inputs) => {
  const maps = {
    price: parseJsonMap(inputs.ModelPrice),
    ratio: parseJsonMap(inputs.ModelRatio),
    cache_ratio: parseJsonMap(inputs.CacheRatio),
    create_cache_ratio: parseJsonMap(inputs.CreateCacheRatio),
    completion_ratio: parseJsonMap(inputs.CompletionRatio),
    image_ratio: parseJsonMap(inputs.ImageRatio),
    audio_ratio: parseJsonMap(inputs.AudioRatio),
    audio_completion_ratio: parseJsonMap(inputs.AudioCompletionRatio),
  };
  const modelNames = new Set(
    Object.values(maps).flatMap((value) => Object.keys(value)),
  );

  return Object.fromEntries(
    Array.from(modelNames).map((modelName) => {
      if (maps.price[modelName] !== undefined) {
        return [
          modelName,
          { mode: 'per-request', price: Number(maps.price[modelName]) },
        ];
      }
      const config = { mode: 'per-token' };
      for (const [field, values] of Object.entries(maps)) {
        if (field !== 'price' && values[modelName] !== undefined) {
          config[field] = Number(values[modelName]);
        }
      }
      return [modelName, config];
    }),
  );
};

export default function ModelRatioSettings({
  options,
  pricingViews = [],
  refresh,
}) {
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ModelPrice: '',
    ModelRatio: '',
    CacheRatio: '',
    CreateCacheRatio: '',
    CompletionRatio: '',
    ImageRatio: '',
    AudioRatio: '',
    AudioCompletionRatio: '',
    ExposeRatioEnabled: false,
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const { t } = useTranslation();

  async function onSubmit() {
    try {
      await refForm.current.validate();
      const desiredByName = buildPricingConfigs(inputs);
      const viewByName = buildPricingViewMap(pricingViews);
      const upserts = Object.entries(desiredByName)
        .filter(([modelName, config]) => {
          const original = viewByName[modelName]?.effective_config;
          return (
            !original ||
            pricingConfigSignature(original) !== pricingConfigSignature(config)
          );
        })
        .map(([modelName, config]) => ({
          model_name: modelName,
          config,
        }));
      const restore = pricingViews
        .filter(
          (view) =>
            view.authority === 'manual' &&
            view.effective_config?.mode !== 'tiered_expr' &&
            desiredByName[view.model_name] === undefined,
        )
        .map((view) => view.model_name);
      const exposeChanged =
        inputs.ExposeRatioEnabled !== inputsRow.ExposeRatioEnabled;

      if (upserts.length === 0 && restore.length === 0 && !exposeChanged) {
        showWarning(t('你似乎并没有修改什么'));
        return;
      }

      setLoading(true);
      if (upserts.length > 0 || restore.length > 0) {
        await saveModelPricingBatch({ upserts, restore });
      }
      if (exposeChanged) {
        const response = await API.put('/api/option/', {
          key: 'ExposeRatioEnabled',
          value: String(inputs.ExposeRatioEnabled),
        });
        if (!response?.data?.success) {
          throw new Error(response?.data?.message || t('保存失败'));
        }
      }
      showSuccess(t('保存成功'));
      await refresh();
    } catch (error) {
      showError(error.message || t('请检查输入'));
      console.error(error);
    } finally {
      setLoading(false);
    }
  }

  async function resetModelRatio() {
    try {
      setLoading(true);
      await calibrateModelPricing();
      showSuccess(t('模型价格校准任务已启动'));
      await refresh();
    } catch (error) {
      showError(error.message || t('模型价格校准失败'));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('模型固定价格')}
              extraText={t('一次调用消耗多少刀，优先级大于模型倍率')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为一次调用消耗多少刀，比如 "gpt-4-gizmo-*": 0.1，一次消耗0.1刀',
              )}
              field={'ModelPrice'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, ModelPrice: value })}
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('模型倍率')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'ModelRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, ModelRatio: value })}
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('提示缓存倍率')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'CacheRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, CacheRatio: value })}
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('缓存创建倍率')}
              extraText={t(
                '默认为 5m 缓存创建倍率；1h 缓存创建倍率按固定乘法自动计算（当前为 1.6x）',
              )}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'CreateCacheRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, CreateCacheRatio: value })
              }
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('模型补全倍率（仅对自定义模型有效）')}
              extraText={t('仅对自定义模型有效')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'CompletionRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, CompletionRatio: value })
              }
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('图片输入倍率（仅部分模型支持该计费）')}
              extraText={t(
                '图片输入相关的倍率设置，键为模型名称，值为倍率，仅部分模型支持该计费',
              )}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gpt-image-1": 2}',
              )}
              field={'ImageRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, ImageRatio: value })}
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('音频倍率（仅部分模型支持该计费）')}
              extraText={t('音频输入相关的倍率设置，键为模型名称，值为倍率')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gpt-4o-audio-preview": 16}',
              )}
              field={'AudioRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, AudioRatio: value })}
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('音频补全倍率（仅部分模型支持该计费）')}
              extraText={t(
                '音频输出补全相关的倍率设置，键为模型名称，值为倍率',
              )}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gpt-4o-realtime": 2}',
              )}
              field={'AudioCompletionRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, AudioCompletionRatio: value })
              }
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col span={16}>
            <Form.Switch
              label={t('暴露倍率接口')}
              field={'ExposeRatioEnabled'}
              onChange={(value) =>
                setInputs({ ...inputs, ExposeRatioEnabled: value })
              }
            />
          </Col>
        </Row>
      </Form>
      <Space>
        <Button onClick={onSubmit}>{t('保存模型倍率设置')}</Button>
        <Popconfirm
          title={t('确定校准模型价格吗？')}
          content={t('将刷新官方价格并重置回退默认值，手动价格保持不变。')}
          okType={'primary'}
          position={'top'}
          onConfirm={resetModelRatio}
        >
          <Button>{t('校准模型价格')}</Button>
        </Popconfirm>
      </Space>
    </Spin>
  );
}
