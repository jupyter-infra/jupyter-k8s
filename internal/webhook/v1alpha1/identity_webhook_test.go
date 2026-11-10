/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/posixidentity"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

const (
	testIdentityPrivateTemplate = "jupyterlab-private"
	testIdentitySharedTemplate  = "jupyterlab-shared"
	testIdentityNamespaceGID    = int64(2130706500)
	testIdentityVolumeGID       = int64(2147000123)
	testTplUserUID              = "{{ .User.Uid }}"
	testTplUserGID              = "{{ .User.Gid }}"
	testTplNamespaceGID         = "{{ .Namespace.Gid }}"
)

func templated(text string) *intstr.IntOrString {
	v := intstr.FromString(text)
	return &v
}

// builderIndexer registers the posixidentity indexes on a fake client builder.
type builderIndexer struct{ builder *fake.ClientBuilder }

func (i *builderIndexer) IndexField(_ context.Context, obj client.Object, field string, fn client.IndexerFunc) error {
	i.builder = i.builder.WithIndex(obj, field, fn)
	return nil
}

var _ = Describe("Identity templates", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		defaulter  WorkspaceCustomDefaulter
		validator  WorkspaceCustomValidator
		privateTpl *workspacev1alpha1.WorkspaceTemplate
		sharedTpl  *workspacev1alpha1.WorkspaceTemplate
	)

	newWorkspace := func(templateName string) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testDefaultNamespace},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: testWorkspaceDisplayName,
				TemplateRef: &workspacev1alpha1.TemplateRef{Name: templateName},
			},
		}
	}

	// admit runs a create through the defaulter then the validator, as the API server would.
	admit := func(ws *workspacev1alpha1.Workspace, user string) error {
		userCtx := createUserContext(ctx, "CREATE", user)
		if err := defaulter.Default(userCtx, ws); err != nil {
			return err
		}
		_, err := validator.ValidateCreate(userCtx, ws)
		return err
	}

	identities := func() []workspacev1alpha1.PosixIdentity {
		list := &workspacev1alpha1.PosixIdentityList{}
		Expect(k8sClient.List(ctx, list)).To(Succeed())
		return list.Items
	}

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		privateTpl = pinnedTemplate(testIdentityPrivateTemplate, testOwnershipOwnerOnly)
		privateTpl.Spec.PodSecurityContextOverrides = lockedSecurityContextPolicy()
		privateTpl.Spec.DefaultPodSecurityContext = &workspacev1alpha1.TemplatedPodSecurityContext{
			RunAsNonRoot:             ptr.To(true),
			SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict),
			FSGroupChangePolicy:      ptr.To(corev1.FSGroupChangeOnRootMismatch),
			RunAsUser:                templated(testTplUserUID),
			RunAsGroup:               templated(testTplUserGID),
			FSGroup:                  templated(testTplUserGID),
			SupplementalGroups:       []intstr.IntOrString{*templated(testTplNamespaceGID), intstr.FromInt32(int32(testIdentityVolumeGID))},
		}
		sharedTpl = pinnedTemplate(testIdentitySharedTemplate, testOwnershipPublic)
		sharedTpl.Spec.DefaultPodSecurityContext = &workspacev1alpha1.TemplatedPodSecurityContext{
			RunAsNonRoot:             ptr.To(true),
			SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict),
			RunAsUser:                templated(testTplNamespaceGID),
			RunAsGroup:               templated(testTplNamespaceGID),
		}

		indexer := &builderIndexer{builder: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			privateTpl, sharedTpl,
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testDefaultNamespace}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   testDefaultNamespace,
				Labels: map[string]string{workspaceutil.LabelPosixGID: strconv.FormatInt(testIdentityNamespaceGID, 10)},
			}},
		)}
		Expect(posixidentity.SetupIndexes(ctx, indexer)).To(Succeed())
		k8sClient = indexer.builder.Build()

		resolver := posixidentity.NewResolver(posixidentity.NewAllocator(k8sClient, k8sClient), k8sClient)
		templateValidator := NewTemplateValidator(k8sClient, "").WithIdentityResolver(resolver)
		defaulter = WorkspaceCustomDefaulter{
			templateDefaulter:       NewTemplateDefaulter(k8sClient, ""),
			identityDefaulter:       NewIdentityDefaulter(k8sClient, "", resolver),
			serviceAccountDefaulter: NewServiceAccountDefaulter(k8sClient),
			templateGetter:          NewTemplateGetter(k8sClient, ""),
			templateValidator:       templateValidator,
			accessStrategyValidator: NewAccessStrategyValidator(""),
			client:                  k8sClient,
		}
		validator = WorkspaceCustomValidator{
			templateValidator:       templateValidator,
			accessStrategyValidator: NewAccessStrategyValidator(""),
			serviceAccountValidator: NewServiceAccountValidator(k8sClient),
			volumeValidator:         NewVolumeValidator(k8sClient),
			storageValidator:        NewStorageValidator(k8sClient),
		}
	})

	It("runs a private workspace as its owner, with the team and volume groups", func() {
		ws := newWorkspace(testIdentityPrivateTemplate)
		Expect(admit(ws, testPinOwner)).To(Succeed())

		Expect(identities()).To(HaveLen(1))
		uid, err := posixidentity.UIDOf(&identities()[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(posixidentity.InUserRange(uid)).To(BeTrue())
		pod := ws.Spec.PodSecurityContext
		Expect(*pod.RunAsUser).To(Equal(uid))
		Expect(*pod.RunAsGroup).To(Equal(uid))
		Expect(*pod.FSGroup).To(Equal(uid))
		Expect(pod.SupplementalGroups).To(Equal([]int64{testIdentityNamespaceGID, testIdentityVolumeGID}))
		Expect(*pod.RunAsNonRoot).To(BeTrue())
		Expect(*pod.SupplementalGroupsPolicy).To(Equal(corev1.SupplementalGroupsPolicyStrict))
		Expect(*pod.FSGroupChangePolicy).To(Equal(corev1.FSGroupChangeOnRootMismatch))
		Expect(ws.Labels[workspaceutil.LabelPosixUID]).To(Equal(strconv.FormatInt(uid, 10)))
	})

	It("gives one user the same uid across workspaces, and two users distinct uids", func() {
		first, second, other := newWorkspace(testIdentityPrivateTemplate), newWorkspace(testIdentityPrivateTemplate), newWorkspace(testIdentityPrivateTemplate)
		second.Name, other.Name = "second", "other"
		Expect(admit(first, testPinOwner)).To(Succeed())
		Expect(admit(second, testPinOwner)).To(Succeed())
		Expect(admit(other, "bob")).To(Succeed())
		Expect(*second.Spec.PodSecurityContext.RunAsUser).To(Equal(*first.Spec.PodSecurityContext.RunAsUser))
		Expect(*other.Spec.PodSecurityContext.RunAsUser).NotTo(Equal(*first.Spec.PodSecurityContext.RunAsUser))
		Expect(identities()).To(HaveLen(2))
	})

	It("runs a shared workspace as the namespace, without allocating a uid", func() {
		ws := newWorkspace(testIdentitySharedTemplate)
		Expect(admit(ws, testPinOwner)).To(Succeed())
		Expect(*ws.Spec.PodSecurityContext.RunAsUser).To(Equal(testIdentityNamespaceGID))
		Expect(*ws.Spec.PodSecurityContext.RunAsGroup).To(Equal(testIdentityNamespaceGID))
		Expect(ws.Labels).NotTo(HaveKey(workspaceutil.LabelPosixUID))
		Expect(identities()).To(BeEmpty())
	})

	It("rejects a workspace picking its own uid", func() {
		ws := newWorkspace(testIdentitySharedTemplate)
		ws.Spec.PodSecurityContext = &corev1.PodSecurityContext{RunAsUser: ptr.To(int64(2047113))}
		Expect(admit(ws, testPinOwner)).To(MatchError(ContainSubstring("spec.podSecurityContext.runAsUser: must be 2130706500")))
	})

	It("rejects a user-supplied posix-uid on create, with an actionable message", func() {
		for _, template := range []string{testIdentitySharedTemplate, testIdentityPrivateTemplate} {
			ws := newWorkspace(template)
			ws.Labels = map[string]string{workspaceutil.LabelPosixUID: "2047113"}
			Expect(admit(ws, testPinOwner)).To(MatchError(
				ContainSubstring("is set by the operator from the owner's PosixIdentity: remove it")))
		}
		Expect(identities()).To(BeEmpty(), "a rejected create allocates nothing")
	})

	It("rejects a posix-uid added by an update", func() {
		oldWorkspace := newWorkspace(testIdentitySharedTemplate)
		Expect(admit(oldWorkspace, testPinOwner)).To(Succeed())
		ws := oldWorkspace.DeepCopy()
		ws.Labels[workspaceutil.LabelPosixUID] = "2047113"
		_, err := validator.ValidateUpdate(createUserContext(ctx, "UPDATE", testPinOwner), oldWorkspace, ws)
		Expect(err).To(MatchError(ContainSubstring("is set by the operator only")))
	})

	It("rejects a workspace in a namespace without a gid label", func() {
		ws := newWorkspace(testIdentitySharedTemplate)
		ws.Namespace = "unlabelled"
		sharedCopy := sharedTpl.DeepCopy()
		sharedCopy.Namespace, sharedCopy.ResourceVersion = "unlabelled", ""
		Expect(k8sClient.Create(ctx, sharedCopy)).To(Succeed())
		Expect(admit(ws, testPinOwner)).To(MatchError(ContainSubstring("has no workspace.jupyter.org/posix-gid label")))
	})

	It("fails closed when the identity defaulter or resolver is not configured", func() {
		ws := newWorkspace(testIdentityPrivateTemplate)
		unwired := defaulter
		unwired.identityDefaulter = nil
		Expect(unwired.Default(createUserContext(ctx, "CREATE", testPinOwner), ws)).To(
			MatchError(ContainSubstring("no identity defaulter is configured")))

		admitted := newWorkspace(testIdentityPrivateTemplate)
		Expect(admit(admitted, testPinOwner)).To(Succeed())
		unchecked := validator
		unchecked.templateValidator = NewTemplateValidator(k8sClient, "")
		_, err := unchecked.ValidateCreate(createUserContext(ctx, "CREATE", testPinOwner), admitted)
		Expect(err).To(MatchError(ContainSubstring("no identity resolver is configured")))
		Expect(identities()).To(HaveLen(1), "only the wired defaulter allocates")
	})

	It("does not allocate on a dry-run create", func() {
		ws := newWorkspace(testIdentityPrivateTemplate)
		userCtx := createUserContext(ctx, "CREATE", testPinOwner)
		req, err := admission.RequestFromContext(userCtx)
		Expect(err).NotTo(HaveOccurred())
		req.DryRun = ptr.To(true)
		userCtx = admission.NewContextWithRequest(ctx, req)
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		_, err = validator.ValidateCreate(userCtx, ws)
		Expect(err).NotTo(HaveOccurred())
		Expect(ws.Spec.PodSecurityContext.RunAsUser).NotTo(BeNil())
		Expect(identities()).To(BeEmpty())
	})

	Context("template consistency", func() {
		It("rejects a template with identity templates but without runAsNonRoot and Strict", func() {
			tpl := privateTpl.DeepCopy()
			tpl.Spec.DefaultPodSecurityContext.RunAsNonRoot = nil
			tpl.Spec.DefaultPodSecurityContext.SupplementalGroupsPolicy = ptr.To(corev1.SupplementalGroupsPolicyMerge)
			Expect(validateTemplateIdentityConsistency(tpl)).To(MatchError(
				ContainSubstring("must set runAsNonRoot: true and supplementalGroupsPolicy: Strict")))
		})

		It("rejects a template referencing an unknown input", func() {
			tpl := privateTpl.DeepCopy()
			tpl.Spec.DefaultPodSecurityContext.RunAsUser = templated("{{ .User.Name }}")
			Expect(validateTemplateIdentityConsistency(tpl)).To(HaveOccurred())
		})

		It("accepts the private and shared recipes", func() {
			Expect(validateTemplateIdentityConsistency(privateTpl)).To(Succeed())
			Expect(validateTemplateIdentityConsistency(sharedTpl)).To(Succeed())
		})
	})
})
