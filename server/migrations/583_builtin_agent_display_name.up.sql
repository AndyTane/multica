-- CLAWSTA-85 batch A: the workspace's built-in Chief of Staff displays as
-- Siman instead of Mika.
--
-- The row is identified by system_key ('mika'), never by name: that key is the
-- server-side identity and stays put, and an owner is free to rename the agent.
-- Only the name the product shipped is migrated, so an owner who already picked
-- another name keeps it.
--
-- The seeded description is product copy that also named the old display name.
-- It is migrated only when it still equals one of the strings the server wrote
-- at creation, and it is rewritten to the exact string the server writes now.
-- A plain REPLACE would not do: in Korean the particle after the display name
-- depends on the name's final consonant, so Mika가 becomes Siman이, not
-- Siman가. The mapping is therefore per language, in both directions.
--
-- Idempotent: after the first run the WHERE matches nothing, so a replayed
-- migration leaves both the rows and their updated_at untouched.
UPDATE agent
SET name = CASE WHEN name = 'Mika' THEN 'Siman' ELSE name END,
    description = CASE description
        WHEN 'Your workspace Chief of Staff. Mika turns goals into issues, coordinates agents, and helps build reusable workflows.'
            THEN 'Your workspace Chief of Staff. Siman turns goals into issues, coordinates agents, and helps build reusable workflows.'
        WHEN '你的工作区 Chief of Staff。Mika 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。'
            THEN '你的工作区 Chief of Staff。Siman 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。'
        WHEN '워크스페이스의 Chief of Staff입니다. Mika가 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.'
            THEN '워크스페이스의 Chief of Staff입니다. Siman이 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.'
        WHEN 'ワークスペースの Chief of Staff。Mika は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
            THEN 'ワークスペースの Chief of Staff。Siman は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
        ELSE description
    END,
    updated_at = now()
WHERE system_key = 'mika'
  AND (
      name = 'Mika'
      OR description IN (
          'Your workspace Chief of Staff. Mika turns goals into issues, coordinates agents, and helps build reusable workflows.',
          '你的工作区 Chief of Staff。Mika 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。',
          '워크스페이스의 Chief of Staff입니다. Mika가 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.',
          'ワークスペースの Chief of Staff。Mika は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
      )
  );
